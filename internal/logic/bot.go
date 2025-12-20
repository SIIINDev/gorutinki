package logic

import (
	"gorutin/internal/domain"
	"math/rand"
	"sort"
	"time"
)

const (
	TileEmpty  = 0
	TileWall   = 1
	TileBox    = 2
	TileBomb   = 3
	TileDanger = 4
	TileAlly   = 5
	TileEnemy  = 6
)

type Bot struct {
	State       *domain.GameState
	Grid        [][]int

	BombRange int
	Speed     int
	MaxBombs  int
	Tick      int

	UnitTargets     map[string]*domain.Vec2d
	UnitExploreDirs map[string]domain.Vec2d
	MemoryTargets   map[domain.Vec2d]int
	AssignedTargets map[domain.Vec2d]string
}

func NewBot() *Bot {
	rand.Seed(time.Now().UnixNano())
	return &Bot{
		BombRange:       1,
		Speed:           2,
		MaxBombs:        1,
		UnitTargets:     make(map[string]*domain.Vec2d),
		UnitExploreDirs: make(map[string]domain.Vec2d),
		MemoryTargets:   make(map[domain.Vec2d]int),
		AssignedTargets: make(map[domain.Vec2d]string),
	}
}

func (b *Bot) UpdateBoosterState(state domain.BoosterState) {
	if state.BombRange > 0 { b.BombRange = state.BombRange }
	if state.Speed > 0 { b.Speed = state.Speed }
	if state.MaxBombs > 0 { b.MaxBombs = state.MaxBombs }
}

func (b *Bot) GetGrid() [][]int {
	if b.Grid == nil && b.State != nil {
		w, h := b.State.MapSize.X(), b.State.MapSize.Y()
		b.Grid = make([][]int, w)
		for x := 0; x < w; x++ { b.Grid[x] = make([]int, h) }
	}
	return b.Grid
}

func (b *Bot) CalculateTurn(state *domain.GameState) *domain.PlayerCommand {
	b.State = state
	b.Tick++

	b.initGrid()
	b.fillGrid()
	b.updateGlobalTargets()
	b.cleanMemory()

	aliveUnits := []domain.Unit{}
	for _, u := range state.MyUnits {
		if u.Alive {
			aliveUnits = append(aliveUnits, u)
		} else {
			b.releaseTarget(u.ID)
		}
	}

	sort.Slice(aliveUnits, func(i, j int) bool {
		return aliveUnits[i].ID < aliveUnits[j].ID
	})

	suicideMode := len(state.MyUnits) > 1 && len(aliveUnits) == 1
	commands := []domain.UnitCommand{}

	for _, unit := range aliveUnits {
		cmd := b.decideUnitAction(unit, suicideMode)
		if cmd != nil {
			commands = append(commands, *cmd)
		}
	}

	if len(commands) == 0 { return nil }
	return &domain.PlayerCommand{Bombers: commands}
}

func (b *Bot) updateGlobalTargets() {
	// Добавляем не сами препятствия, а точки рядом с ними
	for _, box := range b.State.Arena.Obstacles {
		for _, n := range b.neighbors(box) {
			if b.isWalkable(n) {
				if score := b.evaluatePos(n); score > 0 {
					b.MemoryTargets[n] = score
				}
			}
		}
	}
	for _, enemy := range b.State.Enemies {
		for _, n := range b.neighbors(enemy.Pos) {
			if b.isWalkable(n) {
				if score := b.evaluatePos(n); score > 0 {
					b.MemoryTargets[n] = score
				}
			}
		}
	}
}

func (b *Bot) releaseTarget(unitID string) {
	if target, exists := b.UnitTargets[unitID]; exists {
		delete(b.AssignedTargets, *target)
		delete(b.UnitTargets, unitID)
	}
}

func (b *Bot) assignTarget(unitID string, target domain.Vec2d) {
	b.UnitTargets[unitID] = &target
	b.AssignedTargets[target] = unitID
}

func (b *Bot) decideUnitAction(u domain.Unit, suicideMode bool) *domain.UnitCommand {
	// 0. ВЫЖИВАНИЕ (Skip if suicideMode)
	if !suicideMode {
		if b.isTileDangerous(u.Pos) {
			b.releaseTarget(u.ID)
			safePath := b.findSafePath(u.Pos)
			if len(safePath) > 1 {
				return &domain.UnitCommand{ID: u.ID, Path: safePath[1:]}
			}
			return nil
		}
	}

	// 1. СКАНИРОВАНИЕ
	b.scanArea(u.Pos)

	// 2. ЦЕЛЬ
	// Если мы уже на цели, но нет бомб - просто стоим и ждем (согласно запросу)
	// При этом Survival (шаг 0) все еще работает и уведет нас, если станет опасно
	target := b.UnitTargets[u.ID]
	if target != nil && u.Pos == *target && u.BombCount == 0 {
		return &domain.UnitCommand{ID: u.ID}
	}

	// Всегда ищем наилучшую цель с учетом текущего положения
	// Это позволяет переключаться на более выгодные позиции (например, 3 ящика вместо 1)
	best := b.pickBestFromMemory(u.Pos, u.ID)
	
	// Если нашли что-то лучшее (или текущей цели нет)
	if best != nil {
		if target == nil || *target != *best {
			b.releaseTarget(u.ID)
			b.assignTarget(u.ID, *best)
			target = best
		}
	} else if target != nil {
		// Если лучших нет, но старая цель исчезла из памяти - сбрасываем
		if _, exists := b.MemoryTargets[*target]; !exists {
			b.releaseTarget(u.ID)
			target = nil
		}
	}

	if target != nil {
		if u.Pos == *target {
			if u.BombCount > 0 {
				escapePath, isSafe := b.getBlastSafePath(u.Pos)
				if isSafe || suicideMode {
					b.simulateLocalBomb(u.Pos)
					b.cleanMemory()
					
					b.releaseTarget(u.ID)
					delete(b.MemoryTargets, *target)

					cmd := domain.UnitCommand{ID: u.ID, Bombs: []domain.Vec2d{u.Pos}}
					if isSafe && !suicideMode && len(escapePath) > 1 {
						cmd.Path = escapePath[1:]
					}
					return &cmd
				} else {
					// Небезопасно ставить бомбу здесь.
					// Удаляем эту точку из целей, чтобы бот нашел другую (например, с другой стороны ящика)
					b.releaseTarget(u.ID)
					delete(b.MemoryTargets, *target)
				}
			} else {
				// У нас нет бомб, но мы на цели. Стоим и ждем.
				return &domain.UnitCommand{ID: u.ID}
			}
			return nil
		}

		path := b.bfsPath(u.Pos, *target)
		if len(path) > 1 {
			return &domain.UnitCommand{ID: u.ID, Path: path[1:]}
		} else {
			b.releaseTarget(u.ID)
		}
	}

	// 3. БЛУЖДАНИЕ
	dir, hasDir := b.UnitExploreDirs[u.ID]
	nextPos := domain.Vec2d{u.Pos.X() + dir.X(), u.Pos.Y() + dir.Y()}
	
	if !hasDir || !b.isWalkable(nextPos) {
		neighbors := b.neighbors(u.Pos)
		validDirs := []domain.Vec2d{}
		for _, n := range neighbors {
			if b.isWalkable(n) {
				validDirs = append(validDirs, domain.Vec2d{n.X() - u.Pos.X(), n.Y() - u.Pos.Y()})
			}
		}

		if len(validDirs) > 0 {
			idx := (b.Tick*13 + int(u.ID[len(u.ID)-1])*7) % len(validDirs)
			dir = validDirs[idx]
			b.UnitExploreDirs[u.ID] = dir
			nextPos = domain.Vec2d{u.Pos.X() + dir.X(), u.Pos.Y() + dir.Y()}
		} else {
			return nil
		}
	}
	return &domain.UnitCommand{ID: u.ID, Path: []domain.Vec2d{nextPos}}
}

func (b *Bot) simulateLocalBomb(pos domain.Vec2d) {
	b.setTile(pos, TileBomb)
	dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	for _, d := range dirs {
		for i := 1; i <= b.BombRange; i++ {
			p := domain.Vec2d{pos.X() + d.X()*i, pos.Y() + d.Y()*i}
			if !b.isValid(p) { break }
			tile := b.Grid[p.X()][p.Y()]
			if tile == TileWall { break }
			
			b.Grid[p.X()][p.Y()] = TileDanger
			
			if tile == TileBox { break }
		}
	}
}

func (b *Bot) scanArea(pos domain.Vec2d) {
	queue := []domain.Vec2d{pos}
	visited := make(map[domain.Vec2d]bool)
	visited[pos] = true
	depth := make(map[domain.Vec2d]int)
	depth[pos] = 0

	maxDepth := 30
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if depth[curr] > maxDepth { continue }

		if score := b.evaluatePos(curr); score > 0 {
			b.MemoryTargets[curr] = score
		}

		for _, n := range b.neighbors(curr) {
			if !visited[n] && b.isWalkable(n) {
				visited[n] = true
				depth[n] = depth[curr] + 1
				queue = append(queue, n)
			}
		}
	}
}

func (b *Bot) evaluatePos(pos domain.Vec2d) int {
	score := 0
	tile := b.Grid[pos.X()][pos.Y()]
	if tile == TileBox { score += 10 }
	for _, e := range b.State.Enemies {
		if b.isInBombLine(e.Pos, pos) { score += 15 } // Снизили с 50 до 15
	}
	if count := b.countObstaclesInBlast(pos); count > 0 { 
		// Квадратичная зависимость: чем больше ящиков, тем несоразмерно выше очков
		// 1 box = 12
		// 2 boxes = 48
		// 3 boxes = 108
		score += count * count * 12 
	}
	return score
}

func (b *Bot) cleanMemory() {
	for pos, _ := range b.MemoryTargets {
		tile := b.Grid[pos.X()][pos.Y()]
		if tile == TileWall || tile == TileBomb || tile == TileDanger {
			delete(b.MemoryTargets, pos)
			continue
		}
		if b.evaluatePos(pos) == 0 {
			delete(b.MemoryTargets, pos)
		}
	}
	for pos, id := range b.AssignedTargets {
		if _, exists := b.MemoryTargets[pos]; !exists {
			delete(b.AssignedTargets, pos)
			delete(b.UnitTargets, id)
		}
	}
}

func (b *Bot) pickBestFromMemory(myPos domain.Vec2d, myID string) *domain.Vec2d {
	var bestTarget *domain.Vec2d
	bestScore := -100000.0 // Start with a very low score
	
	currentTarget := b.UnitTargets[myID]

	for pos, memScore := range b.MemoryTargets {
		if assignedID, exists := b.AssignedTargets[pos]; exists && assignedID != myID { continue }
		
		dist := b.manhattan(myPos, pos)
		// Removed the check that set dist=1 if dist=0. 
		// We want to prefer the tile we are standing on (dist=0).
		
		finalScore := float64(memScore*10) - float64(dist)

		// Hysteresis: slightly prefer the current target to avoid jitter when scores are close/equal
		if currentTarget != nil && *currentTarget == pos {
			finalScore += 0.5
		}

		if finalScore > bestScore {
			bestScore = finalScore
			cpy := pos
			bestTarget = &cpy
		}
	}
	return bestTarget
}

// ... Grid & Helpers ...

func (b *Bot) initGrid() {
	w, h := b.State.MapSize.X(), b.State.MapSize.Y()
	b.Grid = make([][]int, w)
	for x := 0; x < w; x++ { b.Grid[x] = make([]int, h) }
}

func (b *Bot) fillGrid() {
	for _, w := range b.State.Arena.Walls { b.setTile(w, TileWall) }
	for _, box := range b.State.Arena.Obstacles { b.setTile(box, TileBox) }
	
	// Effective Timer Calc
	effTimers := b.calculateEffectiveTimers()

	for _, bomb := range b.State.Arena.Bombs {
		b.setTile(bomb.Pos, TileBomb)
		b.markDanger(bomb, effTimers[bomb.Pos])
	}
	for _, ally := range b.State.MyUnits {
		if ally.Alive { b.setTile(ally.Pos, TileAlly) }
	}
	for _, enemy := range b.State.Enemies { b.setTile(enemy.Pos, TileEnemy) }
}

func (b *Bot) calculateEffectiveTimers() map[domain.Vec2d]float64 {
	timers := make(map[domain.Vec2d]float64)
	for _, bomb := range b.State.Arena.Bombs {
		timers[bomb.Pos] = bomb.Timer
	}

	for k := 0; k < 3; k++ {
		changed := false
		for _, bomb := range b.State.Arena.Bombs {
			t := timers[bomb.Pos]
			dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
			for _, d := range dirs {
				for i := 1; i <= bomb.Radius; i++ {
					pos := domain.Vec2d{bomb.Pos.X() + d.X()*i, bomb.Pos.Y() + d.Y()*i}
					if !b.isValid(pos) { break }
					if b.Grid[pos.X()][pos.Y()] == TileWall { break }
					
					if otherTimer, ok := timers[pos]; ok {
						if t < otherTimer {
							timers[pos] = t
							changed = true
						}
					}
					if b.Grid[pos.X()][pos.Y()] == TileBox { break }
				}
			}
		}
		if !changed { break }
	}
	return timers
}

func (b *Bot) markDanger(bomb domain.Bomb, timer float64) {
	isCritical := timer <= 3.0
	if isCritical { b.setDanger(bomb.Pos) }
	
	dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	for _, d := range dirs {
		for i := 1; i <= bomb.Radius; i++ {
			pos := domain.Vec2d{bomb.Pos.X() + d.X()*i, bomb.Pos.Y() + d.Y()*i}
			if !b.isValid(pos) { break }
			tile := b.Grid[pos.X()][pos.Y()]
			if tile == TileWall { break }
			if tile == TileBox || tile == TileBomb {
				if isCritical { b.setDanger(pos) }
				break
			}
			if isCritical { b.setDanger(pos) }
		}
	}
}

func (b *Bot) setTile(p domain.Vec2d, val int) { if b.isValid(p) { b.Grid[p.X()][p.Y()] = val } }
func (b *Bot) setDanger(p domain.Vec2d) { if b.isValid(p) && b.Grid[p.X()][p.Y()] != TileWall { b.Grid[p.X()][p.Y()] = TileDanger } }
func (b *Bot) isValid(p domain.Vec2d) bool { return p.X() >= 0 && p.Y() >= 0 && p.X() < b.State.MapSize.X() && p.Y() < b.State.MapSize.Y() }
func (b *Bot) isWalkable(p domain.Vec2d) bool {
	if !b.isValid(p) { return false }
	t := b.Grid[p.X()][p.Y()]
	return t == TileEmpty || t == TileDanger || t == TileAlly
}
func (b *Bot) isTileDangerous(p domain.Vec2d) bool {
	if !b.isValid(p) { return false }
	return b.Grid[p.X()][p.Y()] == TileDanger
}
func (b *Bot) neighbors(p domain.Vec2d) []domain.Vec2d {
	return []domain.Vec2d{{p.X() + 1, p.Y()}, {p.X() - 1, p.Y()}, {p.X(), p.Y() + 1}, {p.X(), p.Y() - 1}}
}
func (b *Bot) manhattan(v1, v2 domain.Vec2d) int { return abs(v1.X()-v2.X()) + abs(v1.Y()-v2.Y()) }

func (b *Bot) bfsPath(start, target domain.Vec2d) []domain.Vec2d {
	if start == target { return []domain.Vec2d{start} }
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr == target { return b.reconstructPath(curr, visited) }
		for _, n := range b.neighbors(curr) {
			if !b.isWalkable(n) { continue }
			if _, seen := visited[n]; !seen {
				visited[n] = curr
				queue = append(queue, n)
			}
		}
	}
	return nil
}

func (b *Bot) findSafePath(start domain.Vec2d) []domain.Vec2d {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if !b.isTileDangerous(curr) { return b.reconstructPath(curr, visited) }
		for _, n := range b.neighbors(curr) {
			if !b.isWalkable(n) { continue }
			if _, seen := visited[n]; !seen {
				visited[n] = curr
				queue = append(queue, n)
			}
		}
	}
	return nil
}

func (b *Bot) getBlastSafePath(pos domain.Vec2d) ([]domain.Vec2d, bool) {
	unsafe := make(map[domain.Vec2d]bool)
	unsafe[pos] = true
	dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	for _, d := range dirs {
		for i := 1; i <= b.BombRange; i++ {
			p := domain.Vec2d{pos.X() + d.X()*i, pos.Y() + d.Y()*i}
			if !b.isValid(p) { break }
			t := b.Grid[p.X()][p.Y()]
			if t == TileWall { break }
			unsafe[p] = true
			if t == TileBox || t == TileBomb { break }
		}
	}
	queue := []domain.Vec2d{pos}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[pos] = domain.Vec2d{-1, -1}
	depth := make(map[domain.Vec2d]int)
	depth[pos] = 0

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if depth[curr] > 10 { continue }
		if !unsafe[curr] && !b.isTileDangerous(curr) {
			return b.reconstructPath(curr, visited), true
		}
		for _, n := range b.neighbors(curr) {
			if !b.isWalkable(n) { continue }
			if _, seen := visited[n]; !seen {
				visited[n] = curr
				depth[n] = depth[curr] + 1
				queue = append(queue, n)
			}
		}
	}
	return nil, false
}

func (b *Bot) reconstructPath(curr domain.Vec2d, visited map[domain.Vec2d]domain.Vec2d) []domain.Vec2d {
	path := []domain.Vec2d{}
	for curr != (domain.Vec2d{-1, -1}) {
		path = append([]domain.Vec2d{curr}, path...)
		curr = visited[curr]
	}
	return path
}

func (b *Bot) countObstaclesInBlast(pos domain.Vec2d) int {
	count := 0
	dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	for _, d := range dirs {
		for i := 1; i <= b.BombRange; i++ {
			p := domain.Vec2d{pos.X() + d.X()*i, pos.Y() + d.Y()*i}
			if !b.isValid(p) { break }
			t := b.Grid[p.X()][p.Y()]
			if t == TileBox { count++; break }
			if t == TileWall || t == TileBomb { break }
		}
	}
	return count
}

func (b *Bot) isInBombLine(target, bomb domain.Vec2d) bool {
	if target == bomb { return true }
	if target.X() == bomb.X() { return abs(target.Y()-bomb.Y()) <= b.BombRange && b.isLineClear(bomb, target) }
	if target.Y() == bomb.Y() { return abs(target.X()-bomb.X()) <= b.BombRange && b.isLineClear(bomb, target) }
	return false
}

func (b *Bot) isLineClear(a, c domain.Vec2d) bool {
	step := domain.Vec2d{sign(c.X() - a.X()), sign(c.Y() - a.Y())}
	curr := domain.Vec2d{a.X() + step.X(), a.Y() + step.Y()}
	for curr != c {
		if !b.isValid(curr) { return false }
		t := b.Grid[curr.X()][curr.Y()]
		if t == TileWall || t == TileBox || t == TileBomb { return false }
		curr = domain.Vec2d{curr.X() + step.X(), curr.Y() + step.Y()}
	}
	return true
}

func abs(x int) int { if x < 0 { return -x }; return x }
func sign(x int) int { if x > 0 { return 1 }; if x < 0 { return -1 }; return 0 }