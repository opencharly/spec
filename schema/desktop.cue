// DESKTOP-SURFACE KINDS — theme, session, displaymanager, desktopentry.
//
// These four describe a graphical session the way `init:`/`service:` describes a supervised
// process: the KIND is a renderer VOCABULARY, and the authored entity is DATA. One theme
// entity renders into a Hyprland config, a sway config and a GTK settings file without
// naming any of them; one session entity expresses a keybinding once for four compositors
// that spell it four ways.
//
// ALL FOUR ARE FLAT, not structural: none deploys or nests a deploy member, so none earns a
// DeployTraits row. They are scalars, templates and small records — the plugin-harness-kind
// shape (skill/hook/marketplace), not the candy/substrate one.
//
// WHY A KIND AND NOT `write:`. Every one of these could be hand-rolled as a `write:` step
// with a heredoc. What a kind buys is a VALIDATOR and CROSS-REFERENCES: a theme template
// that names a token the theme does not define is rejected at load rather than rendering an
// empty colour; a displaymanager naming a session file nothing installs is a black screen at
// boot; a desktopentry's window rules are consumed BY the session renderer, so one entity
// feeds both the applications menu and the compositor's window rules. A `write:` step can
// express none of that.

// ─── theme ────────────────────────────────────────────────────────────────────────────

// #Theme is a colour scheme plus the per-application files it renders into.
//
// The token vocabulary is CLOSED. A template referencing {{.Token.accnt}} is a typo that
// would render an empty colour — a subtly wrong desktop rather than a failure — so
// OpValidate rejects any template naming a token this entity does not define. That check is
// the whole reason this is a kind.
//
// Nothing here names a compositor, so the same entity serves Hyprland, sway, labwc and KDE.
#Theme: {
	// name is the theme's identity, e.g. "tokyo-night". Lowercase and hyphenated, matching
	// what a theme-switcher command accepts.
	name!: string & =~"^[a-z0-9]+(-[a-z0-9]+)*$"

	// variant tells applications which system palette to pair with. Closed: these are the
	// only two values the freedesktop colour-scheme preference has.
	variant?: *"dark" | "light"

	// token is the colour palette. accent, foreground and background are REQUIRED because a
	// theme that defines none of them cannot render anything legible; everything else is
	// optional so a minimal theme stays small.
	token!: #ThemeTokens

	// font, cursor_theme and icon_theme are the non-colour half of a theme. They are named
	// separately from token because they are resource NAMES, not colours, and a template
	// interpolating them into a font stack must not be able to reach the colour namespace.
	font?:         string & !=""
	cursor_theme?: string & !=""
	icon_theme?:   string & !=""

	// background lists wallpaper files, candy-relative. First entry is the default. These
	// lower to `copy:` rather than `write:` — a JPEG base64'd through a plan is neither
	// readable nor small.
	background?: [...string & !=""]

	// render is where the theme becomes files. Each entry names an application and the file
	// to write for it, as a Go template over {{.Token.*}}, {{.Font}}, {{.CursorTheme}},
	// {{.IconTheme}} and {{.Variant}}.
	render?: [...#ThemeRender]
}

// #ThemeTokens is the closed colour vocabulary. Every value is a #HexColor: a token that
// accepted arbitrary strings would let "blue" through and render a broken config.
#ThemeTokens: {
	accent!:     #HexColor
	foreground!: #HexColor
	background!: #HexColor

	cursor?:              #HexColor
	selection_foreground?: #HexColor @go(SelectionForeground)
	selection_background?: #HexColor @go(SelectionBackground)

	// The 16 ANSI slots a terminal palette needs. Named individually rather than as a list
	// so a template says {{.Token.color4}} and a missing one is a load error, not an index
	// panic at render time.
	color0?:  #HexColor
	color1?:  #HexColor
	color2?:  #HexColor
	color3?:  #HexColor
	color4?:  #HexColor
	color5?:  #HexColor
	color6?:  #HexColor
	color7?:  #HexColor
	color8?:  #HexColor
	color9?:  #HexColor
	color10?: #HexColor
	color11?: #HexColor
	color12?: #HexColor
	color13?: #HexColor
	color14?: #HexColor
	color15?: #HexColor
}

// #HexColor is #rrggbb. Six digits only: a renderer that also accepted #rgb or #rrggbbaa
// would have to normalise, and every consuming format spells alpha differently.
#HexColor: string & =~"^#[0-9a-fA-F]{6}$"

// #ThemeRender is one file a theme writes.
#ThemeRender: {
	// app is a label for diagnostics — "foot", "btop", "hyprland". It is not looked up.
	app!: string & !=""
	// path is where the file lands, ${HOME}-relative or absolute.
	path!:    string & !=""
	content!: string
	mode?:    string & =~"^0[0-7]{3,4}$"
	// scope selects whose file it is; user means run_as the image user.
	scope?: *"user" | "system"
	// distro restricts this file to matching distro TAGS, for the cases where one desktop
	// spells a path differently across distros.
	//
	// [...string], not [...#DistroID], and deliberately: these are the same free-form TAGS
	// a candy's `distro:` list carries (box.cue does the same), not the closed guest-distro
	// id a VM source is validated against. #DistroID is also `@go(-)` — it generates no Go
	// type — so a list of it would emit a reference to a type that does not exist.
	distro?: [...(string & !="")]
}

// ─── session ──────────────────────────────────────────────────────────────────────────

// #Session is a compositor's RENDERER VOCABULARY — the exact `init:`/`service:` mirror.
//
// The kind says HOW a construct is spelled; the candy's `desktop:` block says WHICH
// constructs exist. That split is what lets one authored keybinding reach Hyprland's Lua,
// sway's config syntax and labwc's XML without the author knowing any of them.
//
// A compositor that cannot express a construct simply omits its template, and entries of
// that kind are dropped with a diagnostic — the same way supervisord ignores `wanted_by`.
#Session: {
	// compositor is the binary this session runs, e.g. "Hyprland", "sway", "labwc".
	compositor!: string & !=""

	// syntax is a label for diagnostics and for choosing an escaping strategy.
	syntax?: *"lua" | "ini" | "sway" | "xml" | "yaml"

	// config_path_template is where the rendered config lands, as a Go template so a
	// compositor that keys off its own name does not need a second field.
	config_path_template!: string & !=""

	// model says whether the constructs concatenate into ONE file (assembly) or each render
	// to its own (file_set). Hyprland and sway are assembly; labwc is a file set.
	model?: *"assembly" | "file_set"

	// The per-construct templates. Each renders once per authored entry. A compositor that
	// has no notion of a construct omits the template.
	monitor_template?: string
	bind_template?:    string
	input_template?:   string
	exec_template?:    string
	env_template?:     string
	rule_template?:    string
	include_template?: string

	// extra_file is for the parts of a session that are not one of the constructs above —
	// a bootstrap loader, a portal config.
	extra_file?: [...#ThemeRender]

	// session_desktop is the /usr/share/wayland-sessions entry a display manager offers.
	// Its `id` is what a displaymanager's `session:` must match, and that cross-check is
	// enforced at load: an autologin naming a session file nothing installed is a black
	// screen at boot with nothing in any log.
	session_desktop?: #SessionDesktop

	// theme_render lets a session pull colours from a theme entity by name, so a compositor
	// config can be themed without the theme knowing the compositor exists.
	theme_render?: [...string & !=""]
}

// #SessionDesktop is the wayland-sessions entry.
#SessionDesktop: {
	id!:   string & =~"^[a-z0-9]+(-[a-z0-9]+)*$"
	name!: string & !=""
	exec!: string & !=""
	type?: *"Application" | "XSession"
}

// ─── displaymanager ───────────────────────────────────────────────────────────────────

// #DisplayManager is the greeter: which one, which session it starts, and whether it logs a
// user in without asking.
//
// The load-time cross-check is the point: `session` must match a #SessionDesktop.id that
// some session entity in the same project declares. An autologin pointing at a session file
// nothing installed produces a black screen and an empty journal, which is the single worst
// failure this surface has.
#DisplayManager: {
	manager: *"sddm" | "greetd" | "gdm" | "ly"

	// session is the #SessionDesktop.id to start. Cross-checked at load.
	session!: string & =~"^[a-z0-9]+(-[a-z0-9]+)*$"

	autologin?: #AutoLogin
	numlock?:   *"none" | "on" | "off"

	// theme names a greeter theme package or directory. Free-form: every greeter spells
	// this differently and none of them validate it either.
	theme?: string & !=""

	// config is the greeter's own files, rendered the same way a theme's are.
	config?: [...#ThemeRender]

	// unit is the systemd unit to enable. Named explicitly rather than derived from
	// `manager`, because a distro may ship it under a different name and guessing would be
	// the "renderer guesses the distro" failure this codebase already recorded.
	unit?: string & !=""
}

#AutoLogin: {
	user!: string & !=""
	// relogin controls whether the greeter logs the user back in after they log out.
	// Default false: an operator who logs out usually means it.
	relogin?: bool
}

// ─── desktopentry ─────────────────────────────────────────────────────────────────────

// #DesktopEntry is a freedesktop .desktop file.
//
// A .desktop file is a trivial INI, and this kind earns its keep on ONE ground: startup_wm_class
// and window are consumed by the SESSION renderer through the render context, so a single
// entity feeds both the applications menu and the compositor's window rules. Without that it
// would be `write:` duplication and should be dropped.
#DesktopEntry: {
	// entry_name is the file stem and the menu label source.
	entry_name!: string & !=""

	// exec and url are mutually exclusive: `url` renders the browser --app=<url> form, which
	// is all a "web app" installer does. Enforced by OpValidate rather than CUE so the error
	// can say which one to remove.
	exec?: string & !=""
	url?:  string & =~"^https?://"

	// browser_arg are extra flags for the url form.
	browser_arg?: [...string & !=""]

	icon?: #DesktopIcon

	// categories is CLOSED to the freedesktop registered main categories. A typo here does
	// not error anywhere — the entry silently vanishes from every menu — which is exactly
	// the class of failure a closed enum exists to catch.
	categories?: [...#DesktopCategory]

	mime_type?: [...string & !=""]

	// startup_wm_class ties the launched window back to this entry. Also read by the session
	// renderer's window rules, which is this kind's reason to exist.
	startup_wm_class?: string & !=""

	window?: #DesktopWindow

	terminal?: bool
	comment?:  string
}

// #DesktopIcon is name XOR source: a stock icon name, or a candy-relative file that lowers
// to `copy:` (an icon base64'd through a plan is neither readable nor small).
#DesktopIcon: {
	name?:   string & !=""
	source?: string & !=""
}

// #DesktopWindow is the placement the session renderer turns into a window rule.
#DesktopWindow: {
	placement?: *"default" | "float" | "tile" | "fullscreen" | "maximize"
	workspace?: string & !=""
	size?:      string & =~"^[0-9]+x[0-9]+$"
}

// #DesktopCategory is the freedesktop registered MAIN categories, verbatim. Closed on
// purpose — see the note on `categories`.
#DesktopCategory: "AudioVideo" | "Audio" | "Video" | "Development" | "Education" |
	"Game" | "Graphics" | "Network" | "Office" | "Science" | "Settings" |
	"System" | "Utility"
