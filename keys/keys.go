package keys

import (
	"github.com/charmbracelet/bubbles/key"
)

type KeyName int

const (
	KeyUp KeyName = iota
	KeyDown
	KeyEnter
	KeyNew
	KeyKill
	KeyQuit
	KeyReview
	KeyPush

	KeyTab        // Tab is a special keybinding for switching between panes.
	KeySubmitName // SubmitName is a special keybinding for submitting the name of a new instance.

	KeyResume
	KeyPrompt // New key for entering a prompt
	KeyHelp   // Key for showing help screen

	// Diff keybindings
	KeyShiftUp
	KeyShiftDown

	// Reorder keybindings
	KeyMoveUp
	KeyMoveDown
	KeyRestart
)

// GlobalKeyStringsMap is a global, immutable map string to keybinding.
var GlobalKeyStringsMap = map[string]KeyName{
	"up":         KeyUp,
	"k":          KeyUp,
	"down":       KeyDown,
	"j":          KeyDown,
	"shift+up":   KeyShiftUp,
	"shift+down": KeyShiftDown,
	"J":          KeyMoveDown,
	"K":          KeyMoveUp,
	"N":          KeyPrompt,
	"enter":      KeyEnter,
	"o":          KeyEnter,
	"n":          KeyNew,
	"D":          KeyKill,
	"q":          KeyQuit,
	"tab":        KeyTab,
	"r":          KeyResume,
	"R":          KeyRestart,
	"?":          KeyHelp,
}

// cyrillicToLatin maps every Cyrillic character to the one the same physical key produces on
// a US layout. Shortcuts are matched by key position rather than by letter, so `В` runs the
// same action as `D` — it is the same key on the keyboard — and a user working in Russian
// does not have to switch layout to drive the UI.
//
// The map covers the whole alphabet, not just the keys bound today, so a binding added later
// works in both layouts without anyone having to remember this file. TestEveryLetterBinding
// IsReachableFromCyrillic is the guard for that.
var cyrillicToLatin = map[string]string{
	// ЙЦУКЕНГШЩЗХЪ
	"й": "q", "ц": "w", "у": "e", "к": "r", "е": "t", "н": "y",
	"г": "u", "ш": "i", "щ": "o", "з": "p", "х": "[", "ъ": "]",
	"Й": "Q", "Ц": "W", "У": "E", "К": "R", "Е": "T", "Н": "Y",
	"Г": "U", "Ш": "I", "Щ": "O", "З": "P", "Х": "{", "Ъ": "}",

	// ФЫВАПРОЛДЖЭ
	"ф": "a", "ы": "s", "в": "d", "а": "f", "п": "g", "р": "h",
	"о": "j", "л": "k", "д": "l", "ж": ";", "э": "'",
	"Ф": "A", "Ы": "S", "В": "D", "А": "F", "П": "G", "Р": "H",
	"О": "J", "Л": "K", "Д": "L", "Ж": ":", "Э": "\"",

	// ЯЧСМИТЬБЮ
	"я": "z", "ч": "x", "с": "c", "м": "v", "и": "b", "т": "n",
	"ь": "m", "б": ",", "ю": ".", "ё": "`",
	"Я": "Z", "Ч": "X", "С": "C", "М": "V", "И": "B", "Т": "N",
	"Ь": "M", "Б": "<", "Ю": ">", "Ё": "~",
}

// ToLatin returns the character the pressed key produces on a US layout, so a key press can
// be compared against a Latin binding whatever layout is active. Anything that is not a
// Cyrillic character — a Latin one, or a named key such as "esc" — is returned unchanged.
//
// Matching by position rather than by letter is what lets the UI keep advertising one set of
// shortcuts: a prompt that says "press n" means the key labelled N, and that key answers to
// it in either layout.
func ToLatin(key string) string {
	if latin, ok := cyrillicToLatin[key]; ok {
		return latin
	}
	return key
}

// GetKeyName resolves a key press to the action it triggers, accepting either layout.
//
// The second return value reports whether the key is bound at all; callers must check it,
// since the zero KeyName is a real action.
func GetKeyName(key string) (KeyName, bool) {
	name, ok := GlobalKeyStringsMap[ToLatin(key)]
	return name, ok
}

// GlobalkeyBindings is a global, immutable map of KeyName tot keybinding.
var GlobalkeyBindings = map[KeyName]key.Binding{
	KeyUp: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	KeyDown: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	KeyShiftUp: key.NewBinding(
		key.WithKeys("shift+up"),
		key.WithHelp("shift+↑", "scroll"),
	),
	KeyShiftDown: key.NewBinding(
		key.WithKeys("shift+down"),
		key.WithHelp("shift+↓", "scroll"),
	),
	KeyEnter: key.NewBinding(
		key.WithKeys("enter", "o"),
		key.WithHelp("↵/o", "open"),
	),
	KeyNew: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	KeyKill: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "kill"),
	),
	KeyHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	KeyQuit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	KeyPrompt: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new with prompt"),
	),
	KeyTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch tab"),
	),
	KeyResume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume"),
	),
	KeyRestart: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "restart"),
	),

	KeyMoveUp: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "move up"),
	),
	KeyMoveDown: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("J", "move down"),
	),

	// -- Special keybindings --

	KeySubmitName: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "submit name"),
	),
}
