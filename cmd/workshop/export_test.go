package main

var (
	CanUnicode           = canUnicode
	ColorTable           = colorTable
	MonoColorTable       = mono
	ColorColorTable      = color
	NoEscColorTable      = noesc
	ColorMixinGetEscapes = (colorMixin).getEscapes

	MaybePresentWarnings  = maybePresentWarnings
	WriteWarningTimestamp = writeWarningTimestamp
)

func MockIsStdoutTTY(t bool) (restore func()) {
	oldIsStdoutTTY := isStdoutTTY
	isStdoutTTY = t
	return func() {
		isStdoutTTY = oldIsStdoutTTY
	}
}

func MockIsStdinTTY(t bool) (restore func()) {
	oldIsStdinTTY := isStdinTTY
	isStdinTTY = t
	return func() {
		isStdinTTY = oldIsStdinTTY
	}
}

func ColorMixin(cmode, umode string) colorMixin {
	return colorMixin{
		Color:        cmode,
		unicodeMixin: unicodeMixin{Unicode: umode},
	}
}

func MockTextEditor(f func(inPath string, inContent []byte) ([]byte, error)) (restore func()) {
	old := runTextEditor
	runTextEditor = f
	return func() {
		runTextEditor = old
	}
}
