package osiris

import "unicode/utf8"

type sourcePos struct {
	line int
	col  int
}

type sourceSpan struct {
	start sourcePos
	end   sourcePos
}

func spanFromToken(tok token) sourceSpan {
	start := sourcePos{line: tok.line, col: tok.col}
	end := start
	if n := utf8.RuneCountInString(tok.text); n > 0 {
		end.col += n
	}
	return sourceSpan{start: start, end: end}
}

func spanFromTokens(start, end token) sourceSpan {
	sp := spanFromToken(start)
	if end.line > 0 || end.col > 0 {
		sp.end = sourcePos{line: end.line, col: end.col + utf8.RuneCountInString(end.text)}
	}
	return sp
}

func spanCover(a, b sourceSpan) sourceSpan {
	if a.start.line == 0 && a.start.col == 0 {
		return b
	}
	if b.start.line == 0 && b.start.col == 0 {
		return a
	}
	out := a
	if lessPos(b.start, out.start) {
		out.start = b.start
	}
	if lessPos(out.end, b.end) {
		out.end = b.end
	}
	return out
}

func lessPos(a, b sourcePos) bool {
	if a.line != b.line {
		return a.line < b.line
	}
	return a.col < b.col
}

func (s sourceSpan) contains(pos sourcePos) bool {
	if s.start.line == 0 && s.start.col == 0 && s.end.line == 0 && s.end.col == 0 {
		return false
	}
	return !lessPos(pos, s.start) && lessPos(pos, s.end)
}
