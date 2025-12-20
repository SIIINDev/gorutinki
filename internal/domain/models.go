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
	Available []Booster    `json:"available"`
	State     BoosterState `json:"state"`
}

type Booster struct {
	// ID мы будем вычислять сами или надеяться что он есть в JSON скрыто. 
	// Пока добавим его для совместимости с logic.
	ID   int    `json:"id"` 
	Cost int    `json:"cost"`
	Type string `json:"type"`
}

type BoosterState struct {
	Points           int  `json:"points"`
	Armor            int  `json:"armor"`
	BombDelay        int  `json:"bomb_delay"`
	BombRange        int  `json:"bomb_range"`
	Bombers          int  `json:"bombers"`
	MaxBombs         int  `json:"bombs"`
	Speed            int  `json:"speed"`
	View             int  `json:"view"`
	CanPassBombs     bool `json:"can_pass_bombs"`
	CanPassObstacles bool `json:"can_pass_obstacles"`
	CanPassWalls     bool `json:"can_pass_walls"`
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

type RoundListResponse struct {
	Rounds []RoundResponse `json:"rounds"`
	Now    string          `json:"now"`
}

type RoundResponse struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
	Duration int   `json:"duration"`
}

// ServerError - стандартная ошибка от API
type ServerError struct {
	ErrCode int    `json:"errCode"`
	Message string `json:"error"`
}

func (e *ServerError) Error() string {
	return e.Message
}
// BoosterCommand - request body for /api/booster
type BoosterCommand struct {
	Booster int `json:"booster"`
}
