// plugins.go declares the plugin registration model: a metadata-only
// registry that each plugin contributes to at App startup. Every field
// but Slug is optional, so a plugin registers only what it uses.

package app

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// PluginRegistration is the per-plugin entry in the App's registry.
// Each plugin contributes exactly one entry, populated inline from
// RegisterRoutes at the plugin's setup point.
type PluginRegistration struct {
	// Slug is the canonical identifier for this plugin. MUST match the
	// owning plugin's exported PluginSlug const so the lookup is
	// symmetric (slug → plugin code, plugin code → slug).
	Slug string

	// HealthCheck is an optional callback returning nil if the plugin is
	// operational, or an error if not. May be nil — not every plugin has
	// a schema or other failable health signal.
	HealthCheck func() error

	// StaticFS is an optional embedded filesystem of plugin-owned static
	// assets. When non-nil, App.mountPluginStatic() registers it with
	// Echo at /static/plugins/<Slug>/. Use echo.MustSubFS(<embed.FS>,
	// "static") at the registration site so the URL doesn't double the
	// "static" dir. nil = no static assets.
	StaticFS fs.FS

	// Widgets are this plugin's on-sight scripts: boot.js fetches a widget's
	// scripts the first time its data-widget mount appears on a page, so a
	// page without the widget, or a campaign with its add-on switched off,
	// never downloads them (ADR-063). nil = nothing loaded on sight.
	Widgets []PluginWidget
}

// PluginWidget names one data-widget mount and the scripts that implement
// it, in load order: helpers first, the script that calls
// Chronicle.register(Name, …) last. A path starting with "/" is a site path
// (shared code under /static/); any other path is relative to the plugin's
// own StaticFS mount.
type PluginWidget struct {
	Name    string
	Scripts []string
}

// registerPlugin appends a registration entry to the App's registry.
// Package-private — called only from RegisterRoutes at each plugin's
// setup point.
func (a *App) registerPlugin(p PluginRegistration) {
	a.registeredPlugins = append(a.registeredPlugins, p)
}

// RegisteredPlugins returns a copy of the App's registry slice, so
// callers can't mutate the App's internal slice.
func (a *App) RegisteredPlugins() []PluginRegistration {
	out := make([]PluginRegistration, len(a.registeredPlugins))
	copy(out, a.registeredPlugins)
	return out
}

// mountPluginStatic registers each registered plugin's StaticFS with Echo
// at /static/plugins/<slug>/. Called from RegisterRoutes after all plugin
// registrations have happened. Plugins with StaticFS == nil are skipped.

// pluginStaticPrefix is the single definition of where a plugin's embedded
// assets are served, so the mount and the host.embedded diagnostic that
// reports it cannot drift apart.
func pluginStaticPrefix(slug string) string { return "/static/plugins/" + slug }

func (a *App) mountPluginStatic() {
	for _, p := range a.registeredPlugins {
		if p.StaticFS == nil {
			continue
		}
		prefix := pluginStaticPrefix(p.Slug)
		// Register the embed FS so layouts.AssetURL can content-hash plugin
		// assets like on-disk ones, instead of busting all of them per deploy.
		layouts.RegisterAssetFS(prefix+"/", p.StaticFS)
		a.Echo.StaticFS(prefix, p.StaticFS)
	}
}

// coreWidgetOwner names core-owned widgets in buildWidgetManifest's
// reports. It is not a plugin slug and has no static mount.
const coreWidgetOwner = "core"

// buildWidgetManifest flattens the on-sight widgets into the widget-name →
// script-URLs map the layout hands to boot.js: core's own widgets first,
// then each plugin's. Core widgets live under /static/, so their paths must
// be site paths. assetURL turns a site path into its served URL
// (layouts.AssetURL, which adds the content hash). A widget with no name or
// scripts, a core script that isn't a site path, or a name a second owner
// also claims, is a wiring mistake: it is left out and reported, and every
// other widget still loads.
func buildWidgetManifest(core []PluginWidget, regs []PluginRegistration, assetURL func(string) string) (map[string][]string, error) {
	out := map[string][]string{}
	owner := map[string]string{}
	var errs []error
	all := make([]PluginRegistration, 0, len(regs)+1)
	coreReg := PluginRegistration{Slug: coreWidgetOwner}
	for _, w := range core {
		if i := slices.IndexFunc(w.Scripts, func(s string) bool { return !strings.HasPrefix(s, "/") }); i >= 0 {
			errs = append(errs, fmt.Errorf("core widget %q: script %q must be a site path", w.Name, w.Scripts[i]))
			continue
		}
		coreReg.Widgets = append(coreReg.Widgets, w)
	}
	all = append(append(all, coreReg), regs...)
	for _, p := range all {
		for _, w := range p.Widgets {
			if w.Name == "" || len(w.Scripts) == 0 {
				errs = append(errs, fmt.Errorf("plugin %q: widget %q needs a name and at least one script", p.Slug, w.Name))
				continue
			}
			if prev, dup := owner[w.Name]; dup {
				errs = append(errs, fmt.Errorf("widget %q is registered by both %q and %q; keeping %q", w.Name, prev, p.Slug, prev))
				continue
			}
			owner[w.Name] = p.Slug
			urls := make([]string, 0, len(w.Scripts))
			for _, s := range w.Scripts {
				if !strings.HasPrefix(s, "/") {
					s = pluginStaticPrefix(p.Slug) + "/" + s
				}
				urls = append(urls, assetURL(s))
			}
			out[w.Name] = urls
		}
	}
	return out, errors.Join(errs...)
}
