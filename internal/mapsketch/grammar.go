package mapsketch

// Grammar is the page-map line grammar as the AI is told it, word for word.
// Parse reads exactly this; keep the two in step.
const Grammar = "A page map is drawn on a fixed grid 40 squares across and 26 down. Points are `x,y` in squares from the top-left (0,0) to the bottom-right (40,26). One thing per line; lines starting with # are skipped; a line that cannot be drawn is left out and the rest is kept.\n\n" +
	"- `background none|dots|grid|hex`: the map's backdrop (own line, optional)\n" +
	"- `room \"Name\" x,y to x,y` (also `area`, `water`, `danger`): a box from one corner to the other; the name is optional\n" +
	"- `wall x,y to x,y` a box of walls, `wall circle x,y radius r` a round wall, `wall x,y x,y x,y ...` a line of wall\n" +
	"- `path \"Name\" x,y x,y ...` (or `road`): a line through the points\n" +
	"- `label \"words\" x,y`: words on the map\n" +
	"- `pin \"Name\" x,y [number | icon <name>] [page \"Page name\"] [dm] [color <c>]: a note`: a marker. `number` draws a numbered marker; `icon` takes location-dot, house, chess-rook, anchor, church, door-open, skull, gem, star, eye, fire or crown (danger, treasure, quest, note and location also work as words and mean skull, gem, star, eye and location-dot). `page` links a page, whose card shows on hover; the note shows when there is no page. `dm` hides the marker from players.\n" +
	"- `color <c>` also works on room, area, water, danger, wall, path and label lines. `<c>` is sky, teal, orchid, rose, green, violet, cyan, lime or a hex like #3366cc (amber shades are refused: amber marks what only the DM sees).\n\n" +
	"```\nbackground grid\nwall circle 20,13 radius 9 color rose\nroom \"The keep\" 18,11 to 22,15\nwater \"The moat\" 26,2 to 30,5\npath \"Road to the gate\" 20,22 20,26\nlabel \"enemy fires all round\" 2,24\npin \"The banner\" 20,9 icon skull dm: Red, on the keep's tower.\npin \"The gate\" 20,22 number page \"Harrowmere Gate\": Terms at dusk.\n```"
