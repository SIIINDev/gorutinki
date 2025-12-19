package domain

// Vec2d - координаты на карте
type Vec2d struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// GameState - полное состояние игры, приходящее от сервера
type GameState struct {
	MapSize Vec2d  `json:"mapSize"`
	MyUnits []Unit `json:"units"` // Предположим, что сервер присылает наши юниты
	Enemies []Unit `json:"enemies"`
	Walls   []Vec2d `json:"walls"`
	Bombs   []Bomb  `json:"bombs"`
	Mobs    []Mob   `json:"mobs"`
	Tick    int     `json:"tick"`
}

// Unit - наш персонаж
type Unit struct {
	ID        string `json:"id"`
	Pos       Vec2d  `json:"pos"`
	Health    int    `json:"hp"`
	BombCount int    `json:"bombCount"` // Сколько бомб сейчас в инвентаре
}

// Bomb - установленная бомба
type Bomb struct {
	Pos       Vec2d `json:"pos"`
	Timer     int   `json:"timer"`
	Radius    int   `json:"radius"`
	OwnerID   string `json:"ownerId"`
}

// Mob - моб (призрак или патрульный)
type Mob struct {
	Pos  Vec2d  `json:"pos"`
	Type string `json:"type"` // "ghost" или "patrol"
}

// Command - команда отправляемая на сервер
type Command struct {
	UnitID string   `json:"unitId"`
	Move   []Vec2d  `json:"move,omitempty"`  // Путь перемещения
	Bomb   []Vec2d  `json:"plant,omitempty"` // Координаты установки бомб
}
