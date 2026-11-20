package entities

// The kinds of small drawing the Customizer's widget list shows beside each
// widget's name (BlockMeta.Picture). Each is a wireframe of what a widget puts
// on a page, drawn by static/js/widgets/widget_pictures.js; the set stays small
// so the list reads as one family of pictures, and every core widget maps to
// the one that fits it. A package widget declares none and gets PictureLines.
const (
	PictureHeading  = "heading"  // a page's or a home's big title
	PictureLines    = "lines"    // text: grey lines of prose
	PictureQuote    = "quote"    // text read out, set off by an edge
	PictureImage    = "image"    // a picture, a cover, a sky
	PictureCard     = "card"     // one featured page or person: picture over words
	PictureFacts    = "facts"    // labels with their values
	PictureSheet    = "sheet"    // a character sheet's boxed scores
	PictureLinks    = "links"    // a list of linked pages
	PictureGrid     = "grid"     // cards on a grid
	PictureFeed     = "feed"     // posts or notes, one under another
	PictureTree     = "tree"     // a family tree
	PictureGraph    = "graph"    // pages joined by their links
	PictureMap      = "map"      // a map with a pin
	PictureCalendar = "calendar" // a month of days
	PictureTimeline = "timeline" // a track with marks along it
	PictureTable    = "table"    // rows and columns
	PictureCounter  = "counter"  // a number, a clock or a count
	PictureSteps    = "steps"    // a checklist of steps
	PictureTags     = "tags"     // chips
	PictureDivider  = "divider"  // a rule across the page
	PictureSearch   = "search"   // a search field
)

// PictureKinds is every drawing widget_pictures.js knows.
var PictureKinds = map[string]bool{
	PictureHeading: true, PictureLines: true, PictureQuote: true, PictureImage: true,
	PictureCard: true, PictureFacts: true, PictureSheet: true, PictureLinks: true,
	PictureGrid: true, PictureFeed: true, PictureTree: true, PictureGraph: true,
	PictureMap: true, PictureCalendar: true, PictureTimeline: true, PictureTable: true,
	PictureCounter: true, PictureSteps: true, PictureTags: true, PictureDivider: true,
	PictureSearch: true,
}
