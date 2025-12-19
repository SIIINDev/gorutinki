package logic

import (
	"gorutin/internal/domain"
	"sort"
)

type Bot struct {
	State     *domain.GameState
	Grid      [][]int // -1: wall/bomb, 0: empty, 1: danger
	BombRange int
	Speed     int
	roundID   string
	roundTick int
	unitSectors map[string]int
	unitTargets map[string]domain.Vec2d
	attackTargets map[string]attackTarget
	currentSelfID string
}

const (
	CellEmpty  = 0
	CellWall   = -1 // Непроходимо
	CellDanger = 2  // Опасно (зона взрыва)
	MaxPathLen = 30
)

const StartAggroTicks = 10

func NewBot() *Bot {
	return &Bot{BombRange: 1, Speed: 2}
}

func (b *Bot) UpdateBoosterState(state domain.BoosterState) {
	if state.BombRange > 0 {
		b.BombRange = state.BombRange
	}
	if state.Speed > 0 {
		b.Speed = state.Speed
	}
}

func (b *Bot) CalculateTurn(state *domain.GameState) *domain.PlayerCommand {
	b.State = state
	if b.roundID != state.Round {
		b.roundID = state.Round
		b.roundTick = 0
		b.attackTargets = nil
	} else {
		b.roundTick++
	}
	b.buildGrid()
	b.assignUnitTargets()

	var unitCmds []domain.UnitCommand

	for _, unit := range state.MyUnits {
		if !unit.Alive {
			continue
		}
		cmd := b.processUnit(unit)
		if cmd != nil {
			unitCmds = append(unitCmds, *cmd)
		}
	}

	if len(unitCmds) == 0 {
		return nil
	}

	return &domain.PlayerCommand{Bombers: unitCmds}
}

// buildGrid строит карту опасностей и препятствий
func (b *Bot) buildGrid() {
	width, height := b.State.MapSize.X(), b.State.MapSize.Y()
	b.Grid = make([][]int, width)
	for x := 0; x < width; x++ {
		b.Grid[x] = make([]int, height)
	}

	// 1. Стены и препятствия
	for _, w := range b.State.Arena.Walls {
		b.setGrid(w, CellWall)
	}
	for _, o := range b.State.Arena.Obstacles {
		b.setGrid(o, CellWall) // Пока считаем ящики стенами, чтобы не врезаться
	}

	// 2. Бомбы (как препятствия) + Зоны взрыва
	for _, bomb := range b.State.Arena.Bombs {
		b.setGrid(bomb.Pos, CellWall)
		b.markExplosionZone(bomb)
	}
	
	// 3. Другие юниты тоже препятствия (чтобы не застревать)
	for _, enemy := range b.State.Enemies {
		b.setGrid(enemy.Pos, CellWall)
	}
	for _, mob := range b.State.Mobs {
		b.setGrid(mob.Pos, CellWall) // От мобов лучше держаться подальше
		// Можно добавить радиус опасности вокруг моба
	}
}

func (b *Bot) setGrid(p domain.Vec2d, val int) {
	if b.isValid(p) {
		// Если клетка уже помечена как стена, не переписываем на опасность (стена важнее)
		if b.Grid[p.X()][p.Y()] == CellWall && val != CellWall {
			return
		}
		b.Grid[p.X()][p.Y()] = val
	}
}

func (b *Bot) markExplosionZone(bomb domain.Bomb) {
	// Крест взрыва
	dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	b.setGrid(bomb.Pos, CellDanger) // Сама бомба опасна

	for _, d := range dirs {
		for i := 1; i <= bomb.Radius; i++ {
			pos := domain.Vec2d{bomb.Pos.X() + d.X()*i, bomb.Pos.Y() + d.Y()*i}
			if !b.isValid(pos) {
				break
			}
			// Если стена - взрыв дальше не идет
			// ВАЖНО: Тут нужно точнее проверять стены. В текущей модели стены и ящики в Grid=-1.
			// По правилам ящики останавливают взрыв, но сами взрываются.
			// Для простоты пока считаем, что зона за стеной безопасна.
			if b.Grid[pos.X()][pos.Y()] == CellWall {
				// Если это ящик (есть в obstacle list), то он опасен (взорвется), но луч дальше не идет
				if b.isObstacle(pos) {
					b.setGrid(pos, CellDanger)
				}
				break
			}
			b.setGrid(pos, CellDanger)
		}
	}
}

func (b *Bot) processUnit(u domain.Unit) *domain.UnitCommand {
	b.currentSelfID = u.ID
	defer func() { b.currentSelfID = "" }()
	myPos := u.Pos
	if b.onlyUnitAlive(u.ID) && u.BombCount > 0 {
		return &domain.UnitCommand{
			ID:    u.ID,
			Bombs: []domain.Vec2d{myPos},
		}
	}

	// 1. Если мы в опасности - бежим!
	if b.Grid[myPos.X()][myPos.Y()] == CellDanger {
		safePath := b.bfs(myPos, func(p domain.Vec2d) bool {
			return b.Grid[p.X()][p.Y()] == CellEmpty
		})
		if len(safePath) > 1 {
			if len(safePath)-1 > MaxPathLen {
				safePath = safePath[:MaxPathLen+1]
			}
			return &domain.UnitCommand{
				ID:   u.ID,
				Path: safePath[1:], // [0] это текущая позиция
			}
		}
	}

	// 2. Пытаемся добраться до клетки рядом с препятствием.
	// Если есть бомбы - ставим и сразу отходим.
	wantBomb := u.BombCount > 0
	var path []domain.Vec2d
	var bombPos *domain.Vec2d
	if b.isStartAggro() {
		path, bombPos = b.findEnemyAttackPlan(myPos, wantBomb, b.BombRange, u.ID, false)
		if len(path) <= 1 {
			path, bombPos = b.findEnemyAttackPlan(myPos, wantBomb, b.BombRange, u.ID, true)
		}
		if len(path) > 1 {
			if len(path)-1 > MaxPathLen {
				return nil
			}
			cmd := domain.UnitCommand{
				ID:   u.ID,
				Path: path[1:],
			}
			if bombPos != nil {
				cmd.Bombs = []domain.Vec2d{*bombPos}
			}
			return &cmd
		}
	}
	path, bombPos = b.findMobAttackPlan(myPos, wantBomb, b.BombRange, u.ID)
	if len(path) > 1 {
		if len(path)-1 > MaxPathLen {
			return nil
		}
		cmd := domain.UnitCommand{
			ID:   u.ID,
			Path: path[1:],
		}
		if bombPos != nil {
			cmd.Bombs = []domain.Vec2d{*bombPos}
		}
		return &cmd
	}
	if wantBomb {
		if cmd := b.followAttackTarget(u, b.BombRange); cmd != nil {
			return cmd
		}
	} else {
		path, bombPos = b.findAttackPlan(myPos, wantBomb, b.BombRange, u.ID)
		if len(path) > 1 {
			if len(path)-1 > MaxPathLen {
				return nil
			}
			cmd := domain.UnitCommand{
				ID:   u.ID,
				Path: path[1:],
			}
			if bombPos != nil {
				cmd.Bombs = []domain.Vec2d{*bombPos}
			}
			return &cmd
		}
	}

	// 3. Если делать нечего - пытаемся разойтись по секторам карты.
	sector := b.unitSector(u.ID)
	if target, ok := b.unitTargets[u.ID]; ok {
		if b.shouldSeparateFromSharedTarget(u.ID, target) {
			fallbackPath := b.pickFallbackMove(myPos, sector, u.ID)
			if len(fallbackPath) > 1 {
				return &domain.UnitCommand{
					ID:   u.ID,
					Path: fallbackPath[1:],
				}
			}
			return nil
		}
		explorePath := b.findExplorePath(myPos, target)
		if len(explorePath) > 1 {
			if len(explorePath)-1 > MaxPathLen {
				return nil
			}
			return &domain.UnitCommand{
				ID:   u.ID,
				Path: explorePath[1:],
			}
		}
	} else {
		target := b.exploreTargetForSector(sector)
		if target != nil {
			explorePath := b.findExplorePath(myPos, *target)
			if len(explorePath) > 1 {
				if len(explorePath)-1 > MaxPathLen {
					return nil
				}
				return &domain.UnitCommand{
					ID:   u.ID,
					Path: explorePath[1:],
				}
			}
		}
	}
	fallbackPath := b.pickFallbackMove(myPos, sector, u.ID)
	if len(fallbackPath) > 1 {
		return &domain.UnitCommand{
			ID:   u.ID,
			Path: fallbackPath[1:],
		}
	}
	return nil
}

// BFS поиск пути
func (b *Bot) GetGrid() [][]int {
	return b.Grid
}

func (b *Bot) bfs(start domain.Vec2d, isTarget func(domain.Vec2d) bool) []domain.Vec2d {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d) // came_from
	visited[start] = domain.Vec2d{-1, -1}

	if isTarget(start) {
		return []domain.Vec2d{start}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if isTarget(current) {
			// Восстанавливаем путь
			path := []domain.Vec2d{}
			curr := current
			for curr != (domain.Vec2d{-1, -1}) {
				path = append([]domain.Vec2d{curr}, path...)
				curr = visited[curr]
			}
			return path
		}

		// Соседи
		neighbors := []domain.Vec2d{
			{current.X() + 1, current.Y()},
			{current.X() - 1, current.Y()},
			{current.X(), current.Y() + 1},
			{current.X(), current.Y() - 1},
		}

		for _, next := range neighbors {
			if !b.isWalkable(next) {
				continue
			}
			
			if _, seen := visited[next]; !seen {
				visited[next] = current
				queue = append(queue, next)
			}
		}
	}
	return nil
}

func (b *Bot) isStartAggro() bool {
	return b.roundTick < StartAggroTicks
}

func (b *Bot) isValid(p domain.Vec2d) bool {
	return p.X() >= 0 && p.Y() >= 0 && p.X() < b.State.MapSize.X() && p.Y() < b.State.MapSize.Y()
}

func (b *Bot) isObstacle(p domain.Vec2d) bool {
	for _, o := range b.State.Arena.Obstacles {
		if o == p {
			return true
		}
	}
	return false
}

func (b *Bot) isWalkable(p domain.Vec2d) bool {
	if !b.isValid(p) {
		return false
	}
	if b.Grid[p.X()][p.Y()] != CellEmpty {
		return false
	}
	if b.isTooCloseToFriendly(p, b.currentSelfID) {
		return false
	}
	return true
}

func (b *Bot) findAttackPlan(start domain.Vec2d, wantBomb bool, bombRange int, selfID string) ([]domain.Vec2d, *domain.Vec2d) {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}
	steps := make(map[domain.Vec2d]int)
	steps[start] = 0

	bestScore := 0
	var bestPath []domain.Vec2d
	var bestBombPos *domain.Vec2d

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentSteps := steps[current]
		if currentSteps > MaxPathLen {
			continue
		}

		if wantBomb {
			score := b.countObstaclesInBlast(current, bombRange)
			if score > 0 {
				if b.hasFriendlyInBlast(current, bombRange, selfID) {
					goto expand
				}
				escapePath := b.findEscapePath(current, bombRange)
				if len(escapePath) > 1 {
					path := b.reconstructPath(current, visited)
					path = append(path, escapePath[1:]...)
					if len(path)-1 <= MaxPathLen {
						if score > bestScore || (score == bestScore && (bestPath == nil || len(path) < len(bestPath))) {
							bestScore = score
							bestPath = path
							bombPos := current
							bestBombPos = &bombPos
						}
					}
				}
			}
		} else if b.hasAdjacentObstacle(current) {
			path := b.reconstructPath(current, visited)
			if len(path)-1 <= MaxPathLen {
				return path, nil
			}
		}

	expand:
		for _, next := range b.neighbors(current) {
			if !b.isWalkable(next) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				steps[next] = currentSteps + 1
				queue = append(queue, next)
			}
		}
	}

	if bestPath != nil {
		return bestPath, bestBombPos
	}
	return nil, nil
}

func (b *Bot) findEnemyAttackPlan(start domain.Vec2d, wantBomb bool, bombRange int, selfID string, allowInvulnerable bool) ([]domain.Vec2d, *domain.Vec2d) {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if b.canHitEnemyFrom(current, bombRange, allowInvulnerable) {
			path := b.reconstructPath(current, visited)
			if !wantBomb {
				if len(path)-1 <= MaxPathLen {
					return path, nil
				}
			} else {
				if b.hasFriendlyInBlast(current, bombRange, selfID) {
					goto expand
				}
				escapePath := b.findEscapePath(current, bombRange)
				if len(escapePath) > 1 {
					path = append(path, escapePath[1:]...)
					if len(path)-1 <= MaxPathLen {
						bombPos := current
						return path, &bombPos
					}
				}
			}
		}

	expand:
		for _, next := range b.neighbors(current) {
			if !b.isWalkable(next) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				queue = append(queue, next)
			}
		}
	}

	return nil, nil
}

func (b *Bot) canHitEnemyFrom(pos domain.Vec2d, bombRange int, allowInvulnerable bool) bool {
	for _, e := range b.State.Enemies {
		if !allowInvulnerable && e.SafeTime > 0 {
			continue
		}
		if b.isInBombLine(e.Pos, pos, bombRange) {
			return true
		}
	}
	return false
}

func (b *Bot) findMobAttackPlan(start domain.Vec2d, wantBomb bool, bombRange int, selfID string) ([]domain.Vec2d, *domain.Vec2d) {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if b.canHitMobFrom(current, bombRange) {
			path := b.reconstructPath(current, visited)
			if !wantBomb {
				if len(path)-1 <= MaxPathLen {
					return path, nil
				}
			} else {
				if b.hasFriendlyInBlast(current, bombRange, selfID) {
					goto expand
				}
				escapePath := b.findEscapePath(current, bombRange)
				if len(escapePath) > 1 {
					path = append(path, escapePath[1:]...)
					if len(path)-1 <= MaxPathLen {
						bombPos := current
						return path, &bombPos
					}
				}
			}
		}

	expand:
		for _, next := range b.neighbors(current) {
			if !b.isWalkable(next) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				queue = append(queue, next)
			}
		}
	}

	return nil, nil
}

func (b *Bot) canHitMobFrom(pos domain.Vec2d, bombRange int) bool {
	for _, m := range b.State.Mobs {
		if m.SafeTime > 0 {
			continue
		}
		if b.isInBombLine(m.Pos, pos, bombRange) {
			return true
		}
	}
	return false
}

func (b *Bot) hasAdjacentObstacle(p domain.Vec2d) bool {
	for _, n := range b.neighbors(p) {
		if b.isObstacle(n) {
			return true
		}
	}
	return false
}

func (b *Bot) neighbors(p domain.Vec2d) []domain.Vec2d {
	return []domain.Vec2d{
		{p.X() + 1, p.Y()},
		{p.X() - 1, p.Y()},
		{p.X(), p.Y() + 1},
		{p.X(), p.Y() - 1},
	}
}

func (b *Bot) reconstructPath(target domain.Vec2d, visited map[domain.Vec2d]domain.Vec2d) []domain.Vec2d {
	path := []domain.Vec2d{}
	curr := target
	for curr != (domain.Vec2d{-1, -1}) {
		path = append([]domain.Vec2d{curr}, path...)
		curr = visited[curr]
	}
	return path
}

func (b *Bot) findEscapePath(start domain.Vec2d, bombRange int) []domain.Vec2d {
	queue := []domain.Vec2d{start}
	steps := make(map[domain.Vec2d]int)
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}
	steps[start] = 0

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentSteps := steps[current]
		arrivalSec := b.stepsToSeconds(currentSteps)
		if !b.isInBombLine(current, start, bombRange) && !b.isUnsafeDueToBombs(current, arrivalSec, start) {
			return b.reconstructPath(current, visited)
		}

		for _, next := range b.neighbors(current) {
			nextSteps := currentSteps + 1
			arrivalSec = b.stepsToSeconds(nextSteps)
			if !b.isWalkableTimed(next, arrivalSec, start) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				steps[next] = nextSteps
				queue = append(queue, next)
			}
		}
	}

	return nil
}

func (b *Bot) hasFriendlyInBlast(bombPos domain.Vec2d, bombRange int, selfID string) bool {
	for _, u := range b.State.MyUnits {
		if !u.Alive || u.ID == selfID {
			continue
		}
		if b.isInBombLine(u.Pos, bombPos, bombRange) {
			return true
		}
	}
	return false
}

func (b *Bot) isInBombLine(pos domain.Vec2d, bombPos domain.Vec2d, bombRange int) bool {
	if pos == bombPos {
		return true
	}
	if pos.X() == bombPos.X() {
		if absIntBot(pos.Y()-bombPos.Y()) > bombRange {
			return false
		}
		return b.isLineClear(bombPos, pos)
	}
	if pos.Y() == bombPos.Y() {
		if absIntBot(pos.X()-bombPos.X()) > bombRange {
			return false
		}
		return b.isLineClear(bombPos, pos)
	}
	return false
}

func (b *Bot) isLineClear(a domain.Vec2d, c domain.Vec2d) bool {
	if a.X() == c.X() {
		step := 1
		if c.Y() < a.Y() {
			step = -1
		}
		for y := a.Y() + step; y != c.Y(); y += step {
			if b.isBlocked(domain.Vec2d{a.X(), y}) {
				return false
			}
		}
		return true
	}
	if a.Y() == c.Y() {
		step := 1
		if c.X() < a.X() {
			step = -1
		}
		for x := a.X() + step; x != c.X(); x += step {
			if b.isBlocked(domain.Vec2d{x, a.Y()}) {
				return false
			}
		}
		return true
	}
	return false
}

func (b *Bot) isBlocked(p domain.Vec2d) bool {
	for _, w := range b.State.Arena.Walls {
		if w == p {
			return true
		}
	}
	for _, o := range b.State.Arena.Obstacles {
		if o == p {
			return true
		}
	}
	for _, bm := range b.State.Arena.Bombs {
		if bm.Pos == p {
			return true
		}
	}
	return false
}

func absIntBot(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (b *Bot) stepsToSeconds(steps int) float64 {
	speed := b.Speed
	if speed <= 0 {
		speed = 2
	}
	return float64(steps) / float64(speed)
}

func (b *Bot) isWalkableTimed(p domain.Vec2d, arrivalSec float64, ownBombPos domain.Vec2d) bool {
	if !b.isValid(p) {
		return false
	}
	if b.Grid[p.X()][p.Y()] == CellWall {
		return false
	}
	if b.isTooCloseToFriendly(p, b.currentSelfID) {
		return false
	}
	return !b.isUnsafeDueToBombs(p, arrivalSec, ownBombPos)
}

func (b *Bot) isUnsafeDueToBombs(p domain.Vec2d, arrivalSec float64, ownBombPos domain.Vec2d) bool {
	for _, bm := range b.State.Arena.Bombs {
		if bm.Pos == ownBombPos {
			continue
		}
		if b.isInBombLine(p, bm.Pos, bm.Radius) && bm.Timer > arrivalSec {
			return true
		}
	}
	return false
}

func (b *Bot) unitSector(id string) int {
	if b.unitSectors != nil {
		if sector, ok := b.unitSectors[id]; ok {
			return sector
		}
	}
	if id == "" {
		return 0
	}
	sum := 0
	for i := 0; i < len(id); i++ {
		sum = (sum*31 + int(id[i])) % 8
	}
	return sum
}

func (b *Bot) countObstaclesInBlast(bombPos domain.Vec2d, bombRange int) int {
	count := 0
	dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}

	for _, d := range dirs {
		for i := 1; i <= bombRange; i++ {
			pos := domain.Vec2d{bombPos.X() + d.X()*i, bombPos.Y() + d.Y()*i}
			if !b.isValid(pos) {
				break
			}
			if b.isObstacle(pos) {
				count++
				break
			}
			if b.isBlocked(pos) {
				break
			}
		}
	}

	return count
}

type attackTarget struct {
	pos   domain.Vec2d
	score int
}

func (b *Bot) followAttackTarget(u domain.Unit, bombRange int) *domain.UnitCommand {
	if b.attackTargets == nil {
		b.attackTargets = make(map[string]attackTarget)
	}
	current, ok := b.attackTargets[u.ID]
	if ok {
		if b.countObstaclesInBlast(current.pos, bombRange) == 0 {
			ok = false
		}
	}
	if !ok {
		if pos, score, ok2 := b.findBestBombTarget(u.Pos, bombRange, u.ID); ok2 {
			current = attackTarget{pos: pos, score: score}
			b.attackTargets[u.ID] = current
		} else {
			return nil
		}
	}
	if b.shouldSeparateFromSharedTarget(u.ID, current.pos) {
		sector := b.unitSector(u.ID)
		fallbackPath := b.pickFallbackMove(u.Pos, sector, u.ID)
		if len(fallbackPath) > 1 {
			return &domain.UnitCommand{
				ID:   u.ID,
				Path: fallbackPath[1:],
			}
		}
		return nil
	}

	path := b.findPathToTarget(u.Pos, current.pos)
	if len(path) > 1 {
		if len(path)-1 > MaxPathLen {
			return nil
		}
		return &domain.UnitCommand{
			ID:   u.ID,
			Path: path[1:],
		}
	}

	if u.Pos != current.pos {
		return nil
	}
	if b.hasFriendlyInBlast(u.Pos, bombRange, u.ID) {
		return nil
	}
	escapePath := b.findEscapePath(u.Pos, bombRange)
	if len(escapePath) <= 1 {
		return nil
	}

	if pos, score, ok2 := b.findBestBombTarget(u.Pos, bombRange, u.ID); ok2 && pos != current.pos {
		b.attackTargets[u.ID] = attackTarget{pos: pos, score: score}
	} else {
		delete(b.attackTargets, u.ID)
	}

	return &domain.UnitCommand{
		ID:    u.ID,
		Path:  escapePath[1:],
		Bombs: []domain.Vec2d{u.Pos},
	}
}

func (b *Bot) findBestBombTarget(start domain.Vec2d, bombRange int, selfID string) (domain.Vec2d, int, bool) {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	steps := make(map[domain.Vec2d]int)
	visited[start] = domain.Vec2d{-1, -1}
	steps[start] = 0

	bestScore := 0
	bestSteps := 0
	bestPos := domain.Vec2d{}
	found := false

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentSteps := steps[current]
		if currentSteps > MaxPathLen {
			continue
		}
		score := b.countObstaclesInBlast(current, bombRange)
		if score > 0 {
			if b.hasFriendlyInBlast(current, bombRange, selfID) {
				goto expand
			}
			escapePath := b.findEscapePath(current, bombRange)
			if len(escapePath) > 1 {
				totalSteps := currentSteps + (len(escapePath) - 1)
				if totalSteps <= MaxPathLen {
					if score > bestScore || (score == bestScore && (!found || totalSteps < bestSteps)) {
						bestScore = score
						bestSteps = totalSteps
						bestPos = current
						found = true
					}
				}
			}
		}

	expand:
		for _, next := range b.neighbors(current) {
			if !b.isWalkable(next) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				steps[next] = currentSteps + 1
				queue = append(queue, next)
			}
		}
	}

	return bestPos, bestScore, found
}

func (b *Bot) findPathToTarget(start, target domain.Vec2d) []domain.Vec2d {
	return b.bfs(start, func(p domain.Vec2d) bool {
		return p == target
	})
}

func (b *Bot) exploreTargetForSector(sector int) *domain.Vec2d {
	if b.State == nil {
		return nil
	}
	w := b.State.MapSize.X()
	h := b.State.MapSize.Y()
	if w <= 0 || h <= 0 {
		return nil
	}
	xMid := w / 2
	yMid := h / 2
	xQ1 := w / 4
	yQ1 := h / 4
	xQ3 := (w * 3) / 4
	yQ3 := (h * 3) / 4
	xEdge := w / 6
	yEdge := h / 6
	switch sector % 8 {
	case 0: // NW
		return &domain.Vec2d{xQ1, yQ1}
	case 1: // NE
		return &domain.Vec2d{xQ3, yQ1}
	case 2: // SW
		return &domain.Vec2d{xQ1, yQ3}
	case 3: // SE
		return &domain.Vec2d{xQ3, yQ3}
	case 4: // N
		return &domain.Vec2d{xMid, yEdge}
	case 5: // S
		return &domain.Vec2d{xMid, h - 1 - yEdge}
	case 6: // W
		return &domain.Vec2d{xEdge, yMid}
	case 7: // E
		return &domain.Vec2d{w - 1 - xEdge, yMid}
	default:
		return &domain.Vec2d{xMid, yMid}
	}
}

func (b *Bot) findExplorePath(start, target domain.Vec2d) []domain.Vec2d {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}

	best := start
	bestDist := absIntBot(start.X()-target.X()) + absIntBot(start.Y()-target.Y())

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		dist := absIntBot(current.X()-target.X()) + absIntBot(current.Y()-target.Y())
		if dist < bestDist {
			bestDist = dist
			best = current
			if bestDist == 0 {
				break
			}
		}

		for _, next := range b.neighbors(current) {
			if !b.isWalkable(next) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				queue = append(queue, next)
			}
		}
	}

	return b.reconstructPath(best, visited)
}

func (b *Bot) pickFallbackMove(start domain.Vec2d, sector int, selfID string) []domain.Vec2d {
	neighbors := b.neighbors(start)
	if len(neighbors) == 0 {
		return nil
	}
	best := domain.Vec2d{}
	bestSet := false
	bestScore := -1
	shift := (sector + b.roundTick) % len(neighbors)
	for i := 0; i < len(neighbors); i++ {
		n := neighbors[(i+shift)%len(neighbors)]
		if !b.isWalkable(n) {
			continue
		}
		score := b.separationScore(n, selfID)
		if score > bestScore {
			bestScore = score
			best = n
			bestSet = true
		}
	}
	if bestSet {
		return []domain.Vec2d{start, best}
	}
	return nil
}

func (b *Bot) onlyUnitAlive(selfID string) bool {
	alive := 0
	for _, u := range b.State.MyUnits {
		if u.Alive {
			alive++
			if alive > 1 {
				return false
			}
		}
	}
	return alive == 1
}

func (b *Bot) separationScore(pos domain.Vec2d, selfID string) int {
	minFriendDist := 999999
	for _, u := range b.State.MyUnits {
		if !u.Alive || u.ID == selfID {
			continue
		}
		d := manhattanBot(pos, u.Pos)
		if d < minFriendDist {
			minFriendDist = d
		}
	}
	if minFriendDist == 999999 {
		minFriendDist = 0
	}
	obstacleDist := b.nearestObstacleDistance(pos)
	if obstacleDist < 0 {
		obstacleDist = 0
	}
	return minFriendDist*1000 - obstacleDist
}

func (b *Bot) nearestObstacleDistance(pos domain.Vec2d) int {
	if b.State == nil || len(b.State.Arena.Obstacles) == 0 {
		return -1
	}
	best := 999999
	for _, o := range b.State.Arena.Obstacles {
		d := manhattanBot(pos, o)
		if d < best {
			best = d
		}
	}
	return best
}

func (b *Bot) assignUnitTargets() {
	if b.State == nil {
		return
	}
	type unitInfo struct {
		id  string
		pos domain.Vec2d
	}
	units := make([]unitInfo, 0, len(b.State.MyUnits))
	for _, u := range b.State.MyUnits {
		if u.Alive {
			units = append(units, unitInfo{id: u.ID, pos: u.Pos})
		}
	}
	if len(units) == 0 {
		b.unitSectors = nil
		b.unitTargets = nil
		return
	}
	sort.Slice(units, func(i, j int) bool {
		return units[i].id < units[j].id
	})

	candidates := b.candidateTargets()
	sectors := make(map[string]int, len(units))
	targets := make(map[string]domain.Vec2d, len(units))
	if len(candidates) == 0 {
		b.unitSectors = nil
		b.unitTargets = nil
		return
	}

	for i, u := range units {
		bestIdx := 0
		bestScore := -1
		for cIdx, c := range candidates {
			minDist := 999999
			for _, t := range targets {
				d := manhattanBot(c, t)
				if d < minDist {
					minDist = d
				}
			}
			if len(targets) == 0 {
				minDist = 999999
			}
			distFromUnit := manhattanBot(u.pos, c)
			score := minDist*10000 + distFromUnit
			if score > bestScore {
				bestScore = score
				bestIdx = cIdx
			}
		}
		sectors[u.id] = bestIdx % 8
		targets[u.id] = candidates[bestIdx]
		if i+1 >= len(candidates) {
			continue
		}
	}
	b.unitSectors = sectors
	b.unitTargets = targets
}

func (b *Bot) candidateTargets() []domain.Vec2d {
	if b.State == nil {
		return nil
	}
	w := b.State.MapSize.X()
	h := b.State.MapSize.Y()
	if w <= 0 || h <= 0 {
		return nil
	}
	xMid := w / 2
	yMid := h / 2
	xQ1 := w / 4
	yQ1 := h / 4
	xQ3 := (w * 3) / 4
	yQ3 := (h * 3) / 4
	xEdge := w / 6
	yEdge := h / 6
	candidates := []domain.Vec2d{
		{xQ1, yQ1},                 // NW
		{xQ3, yQ1},                 // NE
		{xQ1, yQ3},                 // SW
		{xQ3, yQ3},                 // SE
		{xMid, yEdge},              // N
		{xMid, h - 1 - yEdge},      // S
		{xEdge, yMid},              // W
		{w - 1 - xEdge, yMid},      // E
	}
	filtered := make([]domain.Vec2d, 0, len(candidates))
	for _, c := range candidates {
		if b.isValid(c) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func manhattanBot(a, b domain.Vec2d) int {
	return absIntBot(a.X()-b.X()) + absIntBot(a.Y()-b.Y())
}

func (b *Bot) isTooCloseToFriendly(pos domain.Vec2d, selfID string) bool {
	if b.State == nil {
		return false
	}
	for _, u := range b.State.MyUnits {
		if !u.Alive || u.ID == selfID {
			continue
		}
		if manhattanBot(pos, u.Pos) < 3 {
			return true
		}
	}
	return false
}

func (b *Bot) shouldSeparateFromSharedTarget(selfID string, target domain.Vec2d) bool {
	for _, u := range b.State.MyUnits {
		if !u.Alive || u.ID == selfID {
			continue
		}
		if t, ok := b.attackTargets[u.ID]; ok && t.pos == target {
			return true
		}
		if ut, ok := b.unitTargets[u.ID]; ok && ut == target {
			return true
		}
	}
	return false
}
