//go:build windows

package win32

import "strings"

// The cases of the live keyboard tests. Every layout is typed on the same
// physical keys (scan codes), so the expectations also show what each layout
// does with them. Expectations follow what Chromium's KeyboardEvent reports
// on Windows for the same keys (key, code, location, ctrlKey/altKey/shiftKey)
// except where a comment says otherwise.

const scApps, scEnd, scInsert = 0xE05D, 0xE04F, 0xE052

// w splits the expectation of a case: its events in fmtKey form, separated
// by " ; ".
func w(events string) []string { return strings.Split(events, " ; ") }

func usCases() []kbdCase {
	return []kbdCase{
		{name: "a", keys: ktap(scA), want: w(`d KeyA "a"|"a" - ; u KeyA "a"|"" -`)},
		{name: "shift+a", keys: chord([]uint16{scLShiftK}, scA), want: w(`d ShiftLeft/l "Shift"|"" S ; d KeyA "A"|"A" S ; u KeyA "A"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "ctrl+a", keys: chord([]uint16{scLCtrlK}, scA), want: w(`d ControlLeft/l "Control"|"" C ; d KeyA "a"|"a" C ; u KeyA "a"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "ctrl+shift+a", keys: chord([]uint16{scLCtrlK, scLShiftK}, scA), want: w(`d ControlLeft/l "Control"|"" C ; d ShiftLeft/l "Shift"|"" SC ; d KeyA "A"|"A" SC ; u KeyA "A"|"" SC ; u ShiftLeft/l "Shift"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "alt+a", keys: chord([]uint16{scLAltK}, scA), want: w(`d AltLeft/l "Alt"|"" A ; d KeyA "a"|"a" A ; u KeyA "a"|"" A ; u AltLeft/l "Alt"|"" -`)},
		{name: "shift+2", keys: chord([]uint16{scLShiftK}, scDigit2), want: w(`d ShiftLeft/l "Shift"|"" S ; d Digit2 "@"|"@" S ; u Digit2 "@"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "enter bs esc tab", keys: seq(ktap(scEnterK), ktap(scBackspace), ktap(scEsc), ktap(scTab)), want: w(`d Enter "Enter"|"" - ; u Enter "Enter"|"" - ; d Backspace "Backspace"|"" - ; u Backspace "Backspace"|"" - ; d Escape "Escape"|"" - ; u Escape "Escape"|"" - ; d Tab "Tab"|"" - ; u Tab "Tab"|"" -`)},
		{name: "space", keys: ktap(scSpaceK), want: w(`d Space " "|" " - ; u Space " "|"" -`)},
		{name: "ctrl+space", keys: chord([]uint16{scLCtrlK}, scSpaceK), want: w(`d ControlLeft/l "Control"|"" C ; d Space " "|" " C ; u Space " "|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "left home delete", keys: seq(ktap(scArrowLeft), ktap(scHome), ktap(scDelete)), want: w(`d ArrowLeft "ArrowLeft"|"" - ; u ArrowLeft "ArrowLeft"|"" - ; d Home "Home"|"" - ; u Home "Home"|"" - ; d Delete "Delete"|"" - ; u Delete "Delete"|"" -`)},
		{name: "f1", keys: ktap(scF1), want: w(`d F1 "F1"|"" - ; u F1 "F1"|"" -`)},
		{name: "context menu", keys: ktap(scApps), want: w(`d ContextMenu "ContextMenu"|"" - ; u ContextMenu "ContextMenu"|"" -`)},
		{name: "caps lock a", notWine: "types A for Shift+a with Caps Lock on", keys: seq(ktap(scCapsLock), ktap(scA), chord([]uint16{scLShiftK}, scA), ktap(scCapsLock)), want: w(`d CapsLock "CapsLock"|"" - ; u CapsLock "CapsLock"|"" - ; d KeyA "A"|"A" - ; u KeyA "A"|"" - ; d ShiftLeft/l "Shift"|"" S ; d KeyA "a"|"a" S ; u KeyA "a"|"" S ; u ShiftLeft/l "Shift"|"" - ; d CapsLock "CapsLock"|"" - ; u CapsLock "CapsLock"|"" -`)},
		{name: "numpad numlock on", numlock: true, keys: seq(ktap(scNumpad1), ktap(scNumpadDot), ktap(scNumpadDiv), ktap(scNumpadMul), ktap(scNumpadAdd), ktap(scNumpadEnter)), want: w(`d Numpad1/n "1"|"1" - ; u Numpad1/n "1"|"" - ; d NumpadDecimal/n "."|"." - ; u NumpadDecimal/n "."|"" - ; d NumpadDivide/n "/"|"/" - ; u NumpadDivide/n "/"|"" - ; d NumpadMultiply/n "*"|"*" - ; u NumpadMultiply/n "*"|"" - ; d NumpadAdd/n "+"|"+" - ; u NumpadAdd/n "+"|"" - ; d NumpadEnter/n "Enter"|"" - ; u NumpadEnter/n "Enter"|"" -`)},
		// Alt+0233 types "é" at the release of Alt (code page 1252), and
		// the digits type nothing (Notepad and Chromium).
		{name: "alt+numpad 0233", numlock: true, keys: chord([]uint16{scLAltK}, scNumpad0, scNumpad2, scNumpad3, scNumpad3), want: w(`d AltLeft/l "Alt"|"" A ; d Numpad0/n "0"|"" A ; u Numpad0/n "0"|"" A ; d Numpad2/n "2"|"" A ; u Numpad2/n "2"|"" A ; d Numpad3/n "3"|"" A ; u Numpad3/n "3"|"" A ; d Numpad3/n "3"|"" A ; u Numpad3/n "3"|"" A ; u AltLeft/l "Alt"|"" - ; d ?0x100e9 "é"|"é" - ; u ?0x100e9 "é"|"" -`)},
		{name: "shift+numpad1 numlock on", numlock: true, keys: chord([]uint16{scLShiftK}, scNumpad1), want: w(`d ShiftLeft/l "Shift"|"" S ; u ShiftLeft/l "Shift"|"" - ; d Numpad1/n "End"|"" - ; u Numpad1/n "End"|"" - ; d ShiftLeft/l "Shift"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "end delete numlock on", numlock: true, keys: seq(ktap(scEnd), ktap(scDelete), ktap(scInsert)), want: w(`d End "End"|"" - ; u End "End"|"" - ; d Delete "Delete"|"" - ; u Delete "Delete"|"" - ; d Insert "Insert"|"" - ; u Insert "Insert"|"" -`)},
		{name: "right alt", keys: ktap(scRAltK), want: w(`d AltRight/r "Alt"|"" A ; u AltRight/r "Alt"|"" -`)},
		{name: "right ctrl", keys: ktap(scRCtrlK), want: w(`d ControlRight/r "Control"|"" C ; u ControlRight/r "Control"|"" -`)},
		{name: "right shift", keys: chord([]uint16{scRShiftK}, scA), want: w(`d ShiftRight/r "Shift"|"" S ; d KeyA "A"|"A" S ; u KeyA "A"|"" S ; u ShiftRight/r "Shift"|"" -`)},
		{name: "lone alt", keys: ktap(scLAltK), want: w(`d AltLeft/l "Alt"|"" A ; u AltLeft/l "Alt"|"" -`)},
		{name: "f10", keys: ktap(0x44), want: w(`d F10 "F10"|"" - ; u F10 "F10"|"" -`)},
		{name: "alt+f4", keys: chord([]uint16{scLAltK}, scF4), want: w(`d AltLeft/l "Alt"|"" A ; close ; u AltLeft/l "Alt"|"" -`)},
		{name: "repeat a", slow: true, keys: seq(kdn(scA), kdn(scA), kdn(scA), kup(scA)), want: w(`d KeyA "a"|"a" - ; r KeyA "a"|"a" - ; r KeyA "a"|"a" - ; u KeyA "a"|"" -`)},
		{name: "repeat shift", slow: true, keys: seq(kdn(scLShiftK), kdn(scLShiftK), kdn(scLShiftK), kup(scLShiftK)), want: w(`d ShiftLeft/l "Shift"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "hold a then shift", slow: true, keys: seq(kdn(scA), kdn(scLShiftK), kdn(scA), kup(scLShiftK), kdn(scA), kup(scA)), want: w(`d KeyA "a"|"a" - ; d ShiftLeft/l "Shift"|"" S ; r KeyA "A"|"A" S ; u ShiftLeft/l "Shift"|"" - ; r KeyA "a"|"a" - ; u KeyA "a"|"" -`)},
		{name: "ctrl+numlock", keys: chord([]uint16{scLCtrlK}, scNumLockK), want: w(`d ControlLeft/l "Control"|"" C ; d NumLock "NumLock"|"" C ; u NumLock "NumLock"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "intl backslash", keys: ktap(scIntlBackslash), want: w(`d IntlBackslash "\\"|"\\" - ; u IntlBackslash "\\"|"" -`)},
		{name: "digits row", keys: seq(ktap(scBackquote), ktap(scDigit1), ktap(scDigit0), ktap(scMinus), ktap(scEqual)), want: w("d Backquote \"`\"|\"`\" - ; u Backquote \"`\"|\"\" - ; d Digit1 \"1\"|\"1\" - ; u Digit1 \"1\"|\"\" - ; d Digit0 \"0\"|\"0\" - ; u Digit0 \"0\"|\"\" - ; d Minus \"-\"|\"-\" - ; u Minus \"-\"|\"\" - ; d Equal \"=\"|\"=\" - ; u Equal \"=\"|\"\" -")},
		{name: "punctuation", keys: seq(ktap(scSemicolon), ktap(scQuoteK), ktap(scComma), ktap(scPeriod), ktap(scSlash), ktap(scBackslash), ktap(scBracketL), ktap(scBracketR)), want: w(`d Semicolon ";"|";" - ; u Semicolon ";"|"" - ; d Quote "'"|"'" - ; u Quote "'"|"" - ; d Comma ","|"," - ; u Comma ","|"" - ; d Period "."|"." - ; u Period "."|"" - ; d Slash "/"|"/" - ; u Slash "/"|"" - ; d Backslash "\\"|"\\" - ; u Backslash "\\"|"" - ; d BracketLeft "["|"[" - ; u BracketLeft "["|"" - ; d BracketRight "]"|"]" - ; u BracketRight "]"|"" -`)},
	}
}

func usIntlCases() []kbdCase {
	return []kbdCase{
		{name: "e", keys: ktap(scE), want: w(`d KeyE "e"|"e" - ; u KeyE "e"|"" -`)},
		{name: "quote e (acute)", keys: seq(ktap(scQuoteK), ktap(scE)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d KeyE "é"|"é" - ; u KeyE "e"|"" -`)},
		{name: "quote x (no composition)", keys: seq(ktap(scQuoteK), ktap(scX)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d KeyX "x"|"'x" - ; u KeyX "x"|"" -`)},
		{name: "quote space", keys: seq(ktap(scQuoteK), ktap(scSpaceK)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d Space "'"|"'" - ; u Space " "|"" -`)},
		{name: "quote quote", keys: seq(ktap(scQuoteK), ktap(scQuoteK)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d Quote "'"|"''" - ; u Quote "Dead"|"" -`)},
		{name: "shift+quote (diaeresis) u", keys: seq(chord([]uint16{scLShiftK}, scQuoteK), ktap(scU)), want: w(`d ShiftLeft/l "Shift"|"" S ; d Quote "Dead"|"" S ; u Quote "Dead"|"" S ; u ShiftLeft/l "Shift"|"" - ; d KeyU "ü"|"ü" - ; u KeyU "u"|"" -`)},
		{name: "backquote a (grave)", keys: seq(ktap(scBackquote), ktap(scA)), want: w(`d Backquote "Dead"|"" - ; u Backquote "Dead"|"" - ; d KeyA "à"|"à" - ; u KeyA "a"|"" -`)},
		{name: "shift+6 a (circumflex)", keys: seq(chord([]uint16{scLShiftK}, scDigit1+5), ktap(scA)), want: w(`d ShiftLeft/l "Shift"|"" S ; d Digit6 "Dead"|"" S ; u Digit6 "Dead"|"" S ; u ShiftLeft/l "Shift"|"" - ; d KeyA "â"|"â" - ; u KeyA "a"|"" -`)},
		{name: "quote shift+a", keys: seq(ktap(scQuoteK), chord([]uint16{scLShiftK}, scA)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d ShiftLeft/l "Shift"|"" S ; d KeyA "Á"|"Á" S ; u KeyA "A"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "quote enter", keys: seq(ktap(scQuoteK), ktap(scEnterK)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d Enter "Enter"|"'" - ; u Enter "Enter"|"" -`)},
		{name: "quote then shift", keys: seq(ktap(scQuoteK), ktap(scLShiftK), ktap(scE)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d ShiftLeft/l "Shift"|"" S ; u ShiftLeft/l "Shift"|"" - ; d KeyE "é"|"é" - ; u KeyE "e"|"" -`)},
		{name: "altgr+e", keys: chord([]uint16{scRAltK}, scE), want: w(`d AltRight/r "AltGraph"|"" - ; d KeyE "é"|"é" - ; u KeyE "é"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr+shift+e", keys: chord([]uint16{scRAltK, scLShiftK}, scE), want: w(`d AltRight/r "AltGraph"|"" - ; d ShiftLeft/l "Shift"|"" S ; d KeyE "É"|"É" S ; u KeyE "É"|"" S ; u ShiftLeft/l "Shift"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr alone", keys: ktap(scRAltK), want: w(`d AltRight/r "AltGraph"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "ctrl+alt+e", keys: chord([]uint16{scLCtrlK, scLAltK}, scE), want: w(`d ControlLeft/l "Control"|"" C ; d AltLeft/l "Alt"|"" CA ; d KeyE "e"|"e" CA ; u KeyE "e"|"" CA ; u AltLeft/l "Alt"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "ctrl+e", keys: chord([]uint16{scLCtrlK}, scE), want: w(`d ControlLeft/l "Control"|"" C ; d KeyE "e"|"e" C ; u KeyE "e"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "ctrl+quote", keys: chord([]uint16{scLCtrlK}, scQuoteK), want: w(`d ControlLeft/l "Control"|"" C ; d Quote "Dead"|"" C ; u Quote "Dead"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "left alt+e", keys: chord([]uint16{scLAltK}, scE), want: w(`d AltLeft/l "Alt"|"" A ; d KeyE "e"|"e" A ; u KeyE "e"|"" A ; u AltLeft/l "Alt"|"" -`)},
	}
}

func germanCases() []kbdCase {
	return []kbdCase{
		{name: "y position types z", keys: ktap(scY), want: w(`d KeyY "z"|"z" - ; u KeyY "z"|"" -`)},
		{name: "z position types y", keys: ktap(scZ), want: w(`d KeyZ "y"|"y" - ; u KeyZ "y"|"" -`)},
		{name: "shift+y position", keys: chord([]uint16{scLShiftK}, scY), want: w(`d ShiftLeft/l "Shift"|"" S ; d KeyY "Z"|"Z" S ; u KeyY "Z"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "altgr+q (@)", keys: chord([]uint16{scRAltK}, scQ), want: w(`d AltRight/r "AltGraph"|"" - ; d KeyQ "@"|"@" - ; u KeyQ "@"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr+e (euro)", keys: chord([]uint16{scRAltK}, scE), want: w(`d AltRight/r "AltGraph"|"" - ; d KeyE "€"|"€" - ; u KeyE "€"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr+7 ({)", keys: chord([]uint16{scRAltK}, scDigit7), want: w(`d AltRight/r "AltGraph"|"" - ; d Digit7 "{"|"{" - ; u Digit7 "{"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr+intl backslash (|)", keys: chord([]uint16{scRAltK}, scIntlBackslash), want: w(`d AltRight/r "AltGraph"|"" - ; d IntlBackslash "|"|"|" - ; u IntlBackslash "|"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr alone", keys: ktap(scRAltK), want: w(`d AltRight/r "AltGraph"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "ctrl+alt+q (AltGr by hand)", keys: chord([]uint16{scLCtrlK, scLAltK}, scQ), want: w(`d ControlLeft/l "Control"|"" C ; d AltLeft/l "Alt"|"" CA ; d KeyQ "q"|"q" CA ; u KeyQ "q"|"" CA ; u AltLeft/l "Alt"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "ctrl+q", keys: chord([]uint16{scLCtrlK}, scQ), want: w(`d ControlLeft/l "Control"|"" C ; d KeyQ "q"|"q" C ; u KeyQ "q"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "left alt+q", keys: chord([]uint16{scLAltK}, scQ), want: w(`d AltLeft/l "Alt"|"" A ; d KeyQ "q"|"q" A ; u KeyQ "q"|"" A ; u AltLeft/l "Alt"|"" -`)},
		{name: "ctrl+altgr+q", slow: true, keys: chord([]uint16{scLCtrlK, scRAltK}, scQ), want: w(`d ControlLeft/l "Control"|"" C ; d AltRight/r "AltGraph"|"" C ; d KeyQ "@"|"@" C ; u KeyQ "@"|"" C ; u AltRight/r "AltGraph"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "backquote (dead circumflex) a", keys: seq(ktap(scBackquote), ktap(scA)), want: w(`d Backquote "Dead"|"" - ; u Backquote "Dead"|"" - ; d KeyA "â"|"â" - ; u KeyA "a"|"" -`)},
		{name: "backquote x", keys: seq(ktap(scBackquote), ktap(scX)), want: w(`d Backquote "Dead"|"" - ; u Backquote "Dead"|"" - ; d KeyX "x"|"^x" - ; u KeyX "x"|"" -`)},
		{name: "backquote space", keys: seq(ktap(scBackquote), ktap(scSpaceK)), want: w(`d Backquote "Dead"|"" - ; u Backquote "Dead"|"" - ; d Space "^"|"^" - ; u Space " "|"" -`)},
		{name: "equal (dead acute) e", keys: seq(ktap(scEqual), ktap(scE)), want: w(`d Equal "Dead"|"" - ; u Equal "Dead"|"" - ; d KeyE "é"|"é" - ; u KeyE "e"|"" -`)},
		{name: "shift+equal (dead grave) e", keys: seq(chord([]uint16{scLShiftK}, scEqual), ktap(scE)), want: w(`d ShiftLeft/l "Shift"|"" S ; d Equal "Dead"|"" S ; u Equal "Dead"|"" S ; u ShiftLeft/l "Shift"|"" - ; d KeyE "è"|"è" - ; u KeyE "e"|"" -`)},
		{name: "minus (sharp s)", keys: ktap(scMinus), want: w(`d Minus "ß"|"ß" - ; u Minus "ß"|"" -`)},
		{name: "semicolon (o umlaut)", keys: ktap(scSemicolon), want: w(`d Semicolon "ö"|"ö" - ; u Semicolon "ö"|"" -`)},
		{name: "intl backslash (<)", keys: ktap(scIntlBackslash), want: w(`d IntlBackslash "<"|"<" - ; u IntlBackslash "<"|"" -`)},
		{name: "shift+7 (/)", keys: chord([]uint16{scLShiftK}, scDigit7), want: w(`d ShiftLeft/l "Shift"|"" S ; d Digit7 "/"|"/" S ; u Digit7 "/"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "caps a", keys: seq(ktap(scCapsLock), ktap(scA), ktap(scCapsLock)), want: w(`d CapsLock "CapsLock"|"" - ; u CapsLock "CapsLock"|"" - ; d KeyA "A"|"A" - ; u KeyA "A"|"" - ; d CapsLock "CapsLock"|"" - ; u CapsLock "CapsLock"|"" -`)},
		{name: "numpad dot", numlock: true, keys: ktap(scNumpadDot), want: w(`d NumpadDecimal/n ","|"," - ; u NumpadDecimal/n ","|"" -`)},
		{name: "hold altgr then q", slow: true, keys: seq(kdn(scRAltK), kdn(scRAltK), kdn(scQ), kdn(scQ), kup(scQ), kup(scRAltK)), want: w(`d AltRight/r "AltGraph"|"" - ; d KeyQ "@"|"@" - ; r KeyQ "@"|"@" - ; u KeyQ "@"|"" - ; u AltRight/r "AltGraph"|"" -`)},
	}
}

func frenchCases() []kbdCase {
	return []kbdCase{
		{name: "q position types a", keys: ktap(scQ), want: w(`d KeyQ "a"|"a" - ; u KeyQ "a"|"" -`)},
		{name: "a position types q", keys: ktap(scA), want: w(`d KeyA "q"|"q" - ; u KeyA "q"|"" -`)},
		{name: "semicolon position types m", keys: ktap(scSemicolon), want: w(`d Semicolon "m"|"m" - ; u Semicolon "m"|"" -`)},
		{name: "m position types comma", keys: ktap(scM), want: w(`d KeyM ","|"," - ; u KeyM ","|"" -`)},
		{name: "digit row unshifted (&)", keys: seq(ktap(scDigit1), ktap(scDigit2), ktap(scDigit0)), want: w(`d Digit1 "&"|"&" - ; u Digit1 "&"|"" - ; d Digit2 "é"|"é" - ; u Digit2 "é"|"" - ; d Digit0 "à"|"à" - ; u Digit0 "à"|"" -`)},
		{name: "digit row shifted", keys: seq(chord([]uint16{scLShiftK}, scDigit1), chord([]uint16{scLShiftK}, scDigit2)), want: w(`d ShiftLeft/l "Shift"|"" S ; d Digit1 "1"|"1" S ; u Digit1 "1"|"" S ; u ShiftLeft/l "Shift"|"" - ; d ShiftLeft/l "Shift"|"" S ; d Digit2 "2"|"2" S ; u Digit2 "2"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "altgr+0 (@)", keys: chord([]uint16{scRAltK}, scDigit0), want: w(`d AltRight/r "AltGraph"|"" - ; d Digit0 "@"|"@" - ; u Digit0 "@"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr+e (euro)", keys: chord([]uint16{scRAltK}, scE), want: w(`d AltRight/r "AltGraph"|"" - ; d KeyE "€"|"€" - ; u KeyE "€"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "ctrl+q (a position)", keys: chord([]uint16{scLCtrlK}, scQ), want: w(`d ControlLeft/l "Control"|"" C ; d KeyQ "a"|"a" C ; u KeyQ "a"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "bracket (dead circumflex) e", keys: seq(ktap(scBracketL), ktap(scE)), want: w(`d BracketLeft "Dead"|"" - ; u BracketLeft "Dead"|"" - ; d KeyE "ê"|"ê" - ; u KeyE "e"|"" -`)},
		{name: "shift+bracket (diaeresis) e", keys: seq(chord([]uint16{scLShiftK}, scBracketL), ktap(scE)), want: w(`d ShiftLeft/l "Shift"|"" S ; d BracketLeft "Dead"|"" S ; u BracketLeft "Dead"|"" S ; u ShiftLeft/l "Shift"|"" - ; d KeyE "ë"|"ë" - ; u KeyE "e"|"" -`)},
		{name: "bracket x", keys: seq(ktap(scBracketL), ktap(scX)), want: w(`d BracketLeft "Dead"|"" - ; u BracketLeft "Dead"|"" - ; d KeyX "x"|"^x" - ; u KeyX "x"|"" -`)},
		{name: "bracket shift+e", keys: seq(ktap(scBracketL), chord([]uint16{scLShiftK}, scE)), want: w(`d BracketLeft "Dead"|"" - ; u BracketLeft "Dead"|"" - ; d ShiftLeft/l "Shift"|"" S ; d KeyE "Ê"|"Ê" S ; u KeyE "E"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "ctrl+shift+q", keys: chord([]uint16{scLCtrlK, scLShiftK}, scQ), want: w(`d ControlLeft/l "Control"|"" C ; d ShiftLeft/l "Shift"|"" SC ; d KeyQ "A"|"A" SC ; u KeyQ "A"|"" SC ; u ShiftLeft/l "Shift"|"" C ; u ControlLeft/l "Control"|"" -`)},
	}
}

func spanishCases() []kbdCase {
	return []kbdCase{
		{name: "semicolon (n tilde)", keys: ktap(scSemicolon), want: w(`d Semicolon "ñ"|"ñ" - ; u Semicolon "ñ"|"" -`)},
		{name: "shift+semicolon", keys: chord([]uint16{scLShiftK}, scSemicolon), want: w(`d ShiftLeft/l "Shift"|"" S ; d Semicolon "Ñ"|"Ñ" S ; u Semicolon "Ñ"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "quote (dead acute) a", keys: seq(ktap(scQuoteK), ktap(scA)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d KeyA "á"|"á" - ; u KeyA "a"|"" -`)},
		{name: "quote shift+a", keys: seq(ktap(scQuoteK), chord([]uint16{scLShiftK}, scA)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d ShiftLeft/l "Shift"|"" S ; d KeyA "Á"|"Á" S ; u KeyA "A"|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "quote x", keys: seq(ktap(scQuoteK), ktap(scX)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d KeyX "x"|"´x" - ; u KeyX "x"|"" -`)},
		{name: "quote space", keys: seq(ktap(scQuoteK), ktap(scSpaceK)), want: w(`d Quote "Dead"|"" - ; u Quote "Dead"|"" - ; d Space "´"|"´" - ; u Space " "|"" -`)},
		{name: "shift+quote (dead diaeresis) u", keys: seq(chord([]uint16{scLShiftK}, scQuoteK), ktap(scU)), want: w(`d ShiftLeft/l "Shift"|"" S ; d Quote "Dead"|"" S ; u Quote "Dead"|"" S ; u ShiftLeft/l "Shift"|"" - ; d KeyU "ü"|"ü" - ; u KeyU "u"|"" -`)},
		{name: "altgr+2 (@)", keys: chord([]uint16{scRAltK}, scDigit2), want: w(`d AltRight/r "AltGraph"|"" - ; d Digit2 "@"|"@" - ; u Digit2 "@"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "altgr+e (euro)", keys: chord([]uint16{scRAltK}, scE), want: w(`d AltRight/r "AltGraph"|"" - ; d KeyE "€"|"€" - ; u KeyE "€"|"" - ; u AltRight/r "AltGraph"|"" -`)},
		{name: "shift+2 (quote)", keys: chord([]uint16{scLShiftK}, scDigit2), want: w(`d ShiftLeft/l "Shift"|"" S ; d Digit2 "\""|"\"" S ; u Digit2 "\""|"" S ; u ShiftLeft/l "Shift"|"" -`)},
		{name: "bracket (dead grave) a", keys: seq(ktap(scBracketL), ktap(scA)), want: w(`d BracketLeft "Dead"|"" - ; u BracketLeft "Dead"|"" - ; d KeyA "à"|"à" - ; u KeyA "a"|"" -`)},
		{name: "ctrl+a", keys: chord([]uint16{scLCtrlK}, scA), want: w(`d ControlLeft/l "Control"|"" C ; d KeyA "a"|"a" C ; u KeyA "a"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "ctrl+semicolon", keys: chord([]uint16{scLCtrlK}, scSemicolon), want: w(`d ControlLeft/l "Control"|"" C ; d Semicolon "ñ"|"ñ" C ; u Semicolon "ñ"|"" C ; u ControlLeft/l "Control"|"" -`)},
		{name: "intl backslash (<)", keys: ktap(scIntlBackslash), want: w(`d IntlBackslash "<"|"<" - ; u IntlBackslash "<"|"" -`)},
		{name: "backquote (o ordinal)", keys: ktap(scBackquote), want: w(`d Backquote "º"|"º" - ; u Backquote "º"|"" -`)},
	}
}
