package armory

// maxShopRoomBytes caps the request body. A full 80-piece, 500-item layout is
// well under this; the cap only stops a client from parking arbitrary data in
// the row.
const maxShopRoomBytes = 64 << 10

// ShopRoomLayout is the typed form of a saved shop room. Layouts are decoded
// into this and re-marshalled, never stored as the client sent them, so the
// stored document can only contain fields and values validated here.
type ShopRoomLayout struct {
	Version     int                         `json:"version"`
	RoomType    string                      `json:"roomType"`
	Setting     string                      `json:"setting"`
	Size        string                      `json:"size"`
	Furniture   string                      `json:"furniture"`
	Decorations string                      `json:"decorations"`
	Palette     string                      `json:"palette"`
	Seeds       ShopRoomSeeds               `json:"seeds"`
	Pieces      []ShopRoomPiece             `json:"pieces"`
	Decor       []ShopRoomDecor             `json:"decor"`
	Items       map[string]ShopRoomItemLook `json:"items"`
	Portrait    *ShopRoomPortrait           `json:"portrait"`
	Lines       []string                    `json:"lines"`
}

// ShopRoomSeeds drive the widget's deterministic procedural layout, so a
// room looks the same for every viewer.
type ShopRoomSeeds struct {
	Room  int `json:"room"`
	Goods int `json:"goods"`
	Deco  int `json:"deco"`
}

// ShopRoomPiece is one placed furniture piece. Coordinates are in room tiles.
type ShopRoomPiece struct {
	ID     int     `json:"id"`
	Kind   string  `json:"kind"`
	Wall   string  `json:"wall"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Off    float64 `json:"off"`
	Len    float64 `json:"len"`
	W      float64 `json:"w"`
	D      float64 `json:"d"`
	Pinned bool    `json:"pinned"`
}

// ShopRoomDecor is a decoration the GM placed by hand: an icon standing on
// one of a furniture piece's spots (Spot indexes that piece's spots in
// drawing order). It is dropped when its piece is gone.
type ShopRoomDecor struct {
	Piece int    `json:"piece"`
	Spot  int    `json:"spot"`
	Icon  string `json:"icon"`
}

// ShopRoomItemLook is how one shop good (a "sells" relation) is drawn. Both
// fields may be empty, meaning the widget picks a default.
type ShopRoomItemLook struct {
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// ShopRoomPortrait positions the shopkeeper portrait, as percentages.
type ShopRoomPortrait struct {
	Left float64 `json:"left"`
	Top  float64 `json:"top"`
}
