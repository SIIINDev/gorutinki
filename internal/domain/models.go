package domain

// Vec2d - координаты [x, y]
type Vec2d [2]int

func (v Vec2d) X() int { return v[0] }
func (v Vec2d) Y() int { return v[1] }

// GameState - состояние мира (/api/arena)
type GameState struct {
	Arena    Arena         `json:"arena"`
	MyUnits  []Unit        `json:"bombers"`
	Enemies  []EnemyUnit   `json:"enemies"`
	Mobs     []Mob         `json:"mobs"`
	MapSize  Vec2d         `json:"map_size"`
	Round    string        `json:"round"`
	RawScore int           `json:"raw_score"`
	Tick     int           `json:"-"` // Внутренний счетчик тиков, если нужно
}

type Arena struct {
	Bombs     []Bomb    `json:"bombs"`
	Obstacles []Vec2d   `json:"obstacles"`
	Walls     []Vec2d   `json:"walls"`
}

type Unit struct {
	ID             string `json:"id"`
	Pos            Vec2d  `json:"pos"`
	Alive          bool   `json:"alive"`
	BombCount      int    `json:"bombs_available"` // bombs_available
	SafeTime       int    `json:"safe_time"`
}

type EnemyUnit struct {
	ID       string `json:"id"`
	Pos      Vec2d  `json:"pos"`
	SafeTime int    `json:"safe_time"`
}

type Mob struct {
	ID       string `json:"id"`
	Pos      Vec2d  `json:"pos"`
	Type     string `json:"type"` // "ghost", "patrol" (предположительно)
	SafeTime int    `json:"safe_time"`
}

type Bomb struct {
	Pos    Vec2d   `json:"pos"`
	Timer  float64 `json:"timer"`
	Radius int     `json:"range"`
}

// AvailableBoosterResponse - response for /api/booster
type AvailableBoosterResponse struct {
	Available []AvailableBooster `json:"available"`
	State     BoosterState       `json:"state"`
}

type AvailableBooster struct {
	Cost int    `json:"cost"`
	Type string `json:"type"`
}

type BoosterState struct {
	Armor            int  `json:"armor"`
	BombDelay        int  `json:"bomb_delay"`
	BombRange        int  `json:"bomb_range"`
	Bombers          int  `json:"bombers"`
	Bombs            int  `json:"bombs"`
	CanPassBombs     bool `json:"can_pass_bombs"`
	CanPassObstacles bool `json:"can_pass_obstacles"`
	CanPassWalls     bool `json:"can_pass_walls"`
	Points           int  `json:"points"`
	Speed            int  `json:"speed"`
	View             int  `json:"view"`
}

// PlayerCommand - структура запроса в /api/move
type PlayerCommand struct {
	Bombers []UnitCommand `json:"bombers"`
}

type UnitCommand struct {
	ID    string  `json:"id"`
	Path  []Vec2d `json:"path,omitempty"`
	Bombs []Vec2d `json:"bombs,omitempty"`
}

// BoosterCommand - request body for /api/booster
type BoosterCommand struct {
	Booster int `json:"booster"`
}
