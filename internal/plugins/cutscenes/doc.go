// Package cutscenes keeps a campaign's cut scenes: short full-screen moments
// the Director pushes onto chosen players' screens, in Chronicle and in
// Foundry. A scene is three separate layers (the background, what is drawn on
// top, the words) and an ending; one play of it to named people is a showing,
// which keeps a copy of the scene as shown so the people it was for can watch
// it again for a while even after the scene changes or is deleted.
//
// The scene is data, never markup: every kind, colour and number is checked
// and clamped here before it is stored, and the browser engine draws words
// with textContent. Showing is a live act of the Director's; the AI may write
// scenes through review but can never show one.
package cutscenes
