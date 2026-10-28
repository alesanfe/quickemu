package qelib

// SendkeyName maps a printable character to its QEMU monitor
// sendkey name — "" when the char has no keymapping. Shared by
// quickemu-qmp --type-text and quickemu --type so both agree on
// the full keymap instead of each carrying a partial copy.
func SendkeyName(ch rune) string {
	key := ""
	switch ch {
	case ' ':
		key = "spc"
	case '\n':
		key = "ret"
	case '\t':
		key = "tab"
	case '.':
		key = "dot"
	case ',':
		key = "comma"
	case '-':
		key = "minus"
	case '=':
		key = "equal"
	case '/':
		key = "slash"
	case ':':
		key = "shift-semicolon"
	case ';':
		key = "semicolon"
	case '\'':
		key = "apostrophe"
	case '"':
		key = "shift-apostrophe"
	case '[':
		key = "bracket_left"
	case ']':
		key = "bracket_right"
	case '\\':
		key = "backslash"
	case '`':
		key = "grave_accent"
	case '!':
		key = "shift-1"
	case '@':
		key = "shift-2"
	case '#':
		key = "shift-3"
	case '$':
		key = "shift-4"
	case '%':
		key = "shift-5"
	case '^':
		key = "shift-6"
	case '&':
		key = "shift-7"
	case '*':
		key = "shift-8"
	case '(':
		key = "shift-9"
	case ')':
		key = "shift-0"
	case '_':
		key = "shift-minus"
	case '+':
		key = "shift-equal"
	case '{':
		key = "shift-bracket_left"
	case '}':
		key = "shift-bracket_right"
	case '|':
		key = "shift-backslash"
	case '~':
		key = "shift-grave_accent"
	case '<':
		key = "shift-comma"
	case '>':
		key = "shift-dot"
	case '?':
		key = "shift-slash"
	default:
		if ch >= 'A' && ch <= 'Z' {
			key = "shift-" + string(ch+32)
		} else if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			key = string(ch)
		}
	}
	return key
}
