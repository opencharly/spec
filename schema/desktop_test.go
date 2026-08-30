package schema_test

import (
	"testing"
)

// These tests reuse unifyDef / unifyDefFinal from box_init_test.go rather than defining a
// second copy: one helper, one concatenation contract (R3).
//
// The CORPUS cases use unifyDef (structural). The TEETH use unifyDefFinal, because
// required-field and list-arity violations only surface under FINAL validation — a
// structural unify accepts a #Theme with no `token` since the field is merely not yet
// concrete. Getting that wrong makes a teeth case pass for the wrong reason, which is worse
// than no test.

// CORPUS: a real Omarchy theme, in the shape a theme entity will actually be authored —
// tokens read from one of the 22 bundled colors.toml files plus the per-app files the
// distribution renders them into.
func TestTheme_OmarchyCorpus(t *testing.T) {
	if err := unifyDef(t, "#Theme", `{
		name:    "tokyo-night"
		variant: "dark"
		token: {
			accent: "#7aa2f7", foreground: "#c0caf5", background: "#1a1b26"
			cursor: "#c0caf5"
			selection_foreground: "#c0caf5", selection_background: "#33467c"
			color0: "#15161e", color1: "#f7768e", color2: "#9ece6a", color3: "#e0af68"
			color4: "#7aa2f7", color5: "#bb9af7", color6: "#7dcfff", color7: "#a9b1d6"
		}
		font:         "JetBrainsMono Nerd Font"
		cursor_theme: "Bibata-Modern-Ice"
		icon_theme:   "Yaru-blue"
		background: ["backgrounds/0-winding-road.jpg"]
		render: [
			{app: "foot",     path: "${HOME}/.config/foot/colors.ini", content: "background={{.Token.background}}"},
			{app: "hyprland", path: "${HOME}/.config/hypr/theme.conf", content: "col.active_border = {{.Token.accent}}", scope: "user"},
			{app: "gtk",      path: "/etc/gtk-3.0/settings.ini", content: "gtk-theme-name={{.IconTheme}}", scope: "system", mode: "0644"},
		]
	}`); err != nil {
		t.Fatalf("the Omarchy theme shape must validate: %v", err)
	}
}

// CORPUS: a minimal theme — three tokens and nothing else. Everything but accent/foreground/
// background is optional, so a small theme stays small.
func TestTheme_MinimalIsValid(t *testing.T) {
	if err := unifyDef(t, "#Theme", `{
		name: "mono"
		token: {accent: "#ffffff", foreground: "#ffffff", background: "#000000"}
	}`); err != nil {
		t.Fatalf("a minimal theme must validate: %v", err)
	}
}

func TestTheme_Teeth(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			"a colour name is not a hex colour",
			`{name: "x", token: {accent: "blue", foreground: "#ffffff", background: "#000000"}}`,
		},
		{
			"three-digit hex is rejected — every consuming format spells short form differently",
			`{name: "x", token: {accent: "#fff", foreground: "#ffffff", background: "#000000"}}`,
		},
		{
			"eight-digit hex is rejected — alpha has no portable spelling",
			`{name: "x", token: {accent: "#ffffffff", foreground: "#ffffff", background: "#000000"}}`,
		},
		{
			"accent is required — a theme without it renders nothing legible",
			`{name: "x", token: {foreground: "#ffffff", background: "#000000"}}`,
		},
		{
			"an unknown token is a typo, not an extension point",
			`{name: "x", token: {accent: "#ffffff", foreground: "#ffffff", background: "#000000", accnt: "#123456"}}`,
		},
		{
			"an uppercase theme name would not round-trip through a theme switcher",
			`{name: "TokyoNight", token: {accent: "#ffffff", foreground: "#ffffff", background: "#000000"}}`,
		},
		{
			"a render entry with no content writes an empty file",
			`{name: "x", token: {accent: "#ffffff", foreground: "#ffffff", background: "#000000"},
			  render: [{app: "foot", path: "/tmp/x"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifyDefFinal(t, "#Theme", tc.src); err == nil {
				t.Fatalf("expected rejection, but it validated: %s", tc.src)
			}
		})
	}
}

// CORPUS: the Hyprland session as Omarchy ships it — Lua syntax, an assembly model, and the
// wayland-sessions entry a display manager offers.
func TestSession_HyprlandCorpus(t *testing.T) {
	if err := unifyDef(t, "#Session", `{
		compositor:           "Hyprland"
		syntax:               "lua"
		config_path_template: "${HOME}/.config/hypr/hyprland.lua"
		model:                "assembly"
		monitor_template: "monitor({name = \"{{.Name}}\", resolution = \"{{.Resolution}}\"})"
		bind_template:    "bind({mods = \"{{.Mods}}\", key = \"{{.Key}}\", dispatcher = \"{{.Action}}\"})"
		exec_template:    "exec_once(\"{{.Command}}\")"
		rule_template:    "windowrule({match = \"class:{{.Class}}\", set = \"{{.Rule}}\"})"
		session_desktop: {id: "hyprland", name: "Hyprland", exec: "uwsm start hyprland.desktop"}
		theme_render: ["tokyo-night"]
	}`); err != nil {
		t.Fatalf("the Hyprland session shape must validate: %v", err)
	}
}

// A compositor that cannot express a construct omits its template. sway has no notion of a
// Hyprland-style dispatcher, so a session declaring only what it supports must validate.
func TestSession_PartialTemplateSetIsValid(t *testing.T) {
	if err := unifyDef(t, "#Session", `{
		compositor:           "labwc"
		syntax:               "xml"
		config_path_template: "${HOME}/.config/labwc/rc.xml"
		model:                "file_set"
		bind_template:        "<keybind key=\"{{.Key}}\"><action name=\"Execute\" command=\"{{.Action}}\"/></keybind>"
	}`); err != nil {
		t.Fatalf("a session declaring only the constructs it supports must validate: %v", err)
	}
}

func TestSession_Teeth(t *testing.T) {
	cases := []struct{ name, src string }{
		{"a session with no compositor cannot be started",
			`{config_path_template: "/tmp/x"}`},
		{"a session with no config path has nowhere to render",
			`{compositor: "sway"}`},
		{"an unknown syntax would pick the wrong escaping",
			`{compositor: "sway", syntax: "toml", config_path_template: "/tmp/x"}`},
		{"an unknown model is neither assembly nor file_set",
			`{compositor: "sway", config_path_template: "/tmp/x", model: "merge"}`},
		{"a session_desktop id must be a slug — a displaymanager cross-checks it",
			`{compositor: "sway", config_path_template: "/tmp/x",
			  session_desktop: {id: "Sway Session", name: "Sway", exec: "sway"}}`},
		{"an unknown field is a typo",
			`{compositor: "sway", config_path_template: "/tmp/x", keybind_template: "x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifyDefFinal(t, "#Session", tc.src); err == nil {
				t.Fatalf("expected rejection, but it validated: %s", tc.src)
			}
		})
	}
}

// CORPUS: sddm with autologin, which is what an Omarchy machine boots into.
func TestDisplayManager_Corpus(t *testing.T) {
	if err := unifyDef(t, "#DisplayManager", `{
		manager:   "sddm"
		session:   "hyprland"
		autologin: {user: "user", relogin: false}
		numlock:   "on"
		theme:     "omarchy"
		unit:      "sddm.service"
		config: [{app: "sddm", path: "/etc/sddm.conf.d/10-charly.conf", content: "[Autologin]\nUser={{.User}}", scope: "system"}]
	}`); err != nil {
		t.Fatalf("the sddm shape must validate: %v", err)
	}
}

func TestDisplayManager_Teeth(t *testing.T) {
	cases := []struct{ name, src string }{
		{"an unknown greeter has no renderer", `{manager: "lightdm", session: "hyprland"}`},
		{"a displaymanager with no session has nothing to start", `{manager: "sddm"}`},
		{"the session must be a slug so the cross-check against #SessionDesktop.id can work",
			`{manager: "sddm", session: "Hyprland Session"}`},
		{"autologin without a user cannot log anyone in",
			`{manager: "sddm", session: "hyprland", autologin: {relogin: true}}`},
		{"an unknown numlock value is a typo", `{manager: "sddm", session: "hyprland", numlock: "yes"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifyDefFinal(t, "#DisplayManager", tc.src); err == nil {
				t.Fatalf("expected rejection, but it validated: %s", tc.src)
			}
		})
	}
}

// CORPUS: both shapes a desktop entry takes — a plain command, and the browser --app= web-app
// form an omarchy-webapp-install produces.
func TestDesktopEntry_Corpus(t *testing.T) {
	if err := unifyDef(t, "#DesktopEntry", `{
		entry_name:       "Disk Usage"
		exec:             "xdg-terminal-exec -e bash -c \"dua i /\""
		icon:             {name: "drive-harddisk"}
		categories:       ["System", "Utility"]
		terminal:         true
		startup_wm_class: "TUI.float"
		window:           {placement: "float", size: "1200x800"}
	}`); err != nil {
		t.Fatalf("the command form must validate: %v", err)
	}
	if err := unifyDef(t, "#DesktopEntry", `{
		entry_name:  "GitHub"
		url:         "https://github.com"
		browser_arg: ["--new-window"]
		icon:        {source: "icons/github.png"}
		categories:  ["Network"]
		window:      {placement: "tile", workspace: "2"}
	}`); err != nil {
		t.Fatalf("the web-app form must validate: %v", err)
	}
}

func TestDesktopEntry_Teeth(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			// The reason categories is a closed enum: a typo does not error anywhere, the
			// entry just silently vanishes from every menu.
			"an unregistered category silently drops the entry from every menu",
			`{entry_name: "X", exec: "x", categories: ["Utilities"]}`,
		},
		{"a non-http url is not something a browser --app= can open",
			`{entry_name: "X", url: "ftp://example.invalid"}`},
		{"an entry with no name has no menu label",
			`{exec: "x"}`},
		{"a size that is not WxH cannot become a window rule",
			`{entry_name: "X", exec: "x", window: {size: "large"}}`},
		{"an unknown placement is a typo",
			`{entry_name: "X", exec: "x", window: {placement: "centre"}}`},
		{"an unknown field is a typo",
			`{entry_name: "X", exec: "x", Categories: ["Utility"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifyDefFinal(t, "#DesktopEntry", tc.src); err == nil {
				t.Fatalf("expected rejection, but it validated: %s", tc.src)
			}
		})
	}
}

// The four defs must exist and be closed. A def that silently accepted unknown fields would
// make every "unknown field is a typo" case above pass for the wrong reason.
func TestDesktopKinds_AreDefinedAndClosed(t *testing.T) {
	for _, def := range []string{"#Theme", "#Session", "#DisplayManager", "#DesktopEntry", "#ThemeTokens", "#DesktopWindow"} {
		t.Run(def, func(t *testing.T) {
			if err := unifyDefFinal(t, def, `{definitely_not_a_field: "x"}`); err == nil {
				t.Fatalf("%s accepted an unknown field — it is not closed", def)
			}
		})
	}
}
