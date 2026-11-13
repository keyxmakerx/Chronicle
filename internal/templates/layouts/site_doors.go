package layouts

import (
	"context"
	"fmt"
)

// Door names the three ways in on the sign-in page. They are also the values
// of the stage's data-door attribute and the tabs', which the scene CSS and
// signin_scene.js key on.
const (
	DoorSignIn = "signin"
	DoorNew    = "new"
	DoorCode   = "code"
)

// doorScene is the built-in scene each door shows.
var doorScenes = []struct{ Door, Scene string }{
	{DoorSignIn, "castle"},
	{DoorNew, "dragon"},
	{DoorCode, "tavern"},
}

// doorLayers are a scene's four drawn layers, back to front.
var doorLayers = []struct{ N, Name string }{
	{"1", "back"}, {"2", "mid"}, {"3", "front"}, {"4", "action"},
}

// doorLayerURL is the static path of one pre-blurred scene layer.
func doorLayerURL(scene, n, name string) string {
	return AssetURL(fmt.Sprintf("/static/img/signin/%s-%s-%s.webp", scene, n, name))
}

// siteDoorsMode is how the stage is dressed. A site that never saved a look
// gets the built-in scenes; an admin's picture or look colours replace them,
// and a saved plain look shows the panel on the page colour.
func siteDoorsMode(ctx context.Context) string {
	switch {
	case siteAuthPicture(ctx) != "":
		return "picture"
	case siteAuthGradient(ctx) != "":
		return "gradient"
	case GetSiteLook(ctx).Configured:
		return "plain"
	}
	return "scenes"
}
