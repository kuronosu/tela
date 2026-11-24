package canvas

// A bound is how much a cell may differ from Chromium's at either scale, in
// mean and at most, in levels of a channel, and why. Those kept differ for
// another reason than how the same shapes are sampled: Tela means it, and one
// that is within the bound of its scene is not a difference any more.
//
// The numbers are what was measured, rounded up. Another version of Chromium
// may need others: look at the cells before changing them.
type bound struct {
	mean float64
	max  int
	why  string
	kept bool
}

// Why the cells differ.
const (
	// Tela covers pixels by exact area. Skia, without a GPU, samples shapes
	// that are not convex in four rows of a pixel, and makes curves of a few
	// parabolas and lines, up to a quarter of a pixel away: the coverage of
	// its circle of 40 px is up to 71 of 255 from the exact one, where that of
	// the same circle made of 3000 lines is within 20 (measured in it).
	whyEdges = "edges: exact areas against Skia's samples and coarse curves"
	// And Skia makes the coverage of glyphs darker (its contrast and gamma for
	// text), by the luminance of their color.
	whyText = "edges, and Skia's contrast for text"
	// What overlaps itself within a path adds its partial coverage in Tela's
	// rasterizer: a line that comes back on itself, and the outlines of
	// variable fonts, stroked or filled.
	whyOverlap = "parts of a path that overlap: their partial coverage adds up"
	whyDashes  = "dashes along a curve: Skia measures them along a few lines, to half a pixel"
	// Skia's software sampler, against weights in 1/256.
	whyWeights  = "the weights of the neighboring pixels of an image, sixteenths in Skia"
	whyRounding = "rounding"
	// And an approximate arctangent in Skia.
	whySeam = "pixels exactly on a hard edge of a conic gradient"
	// The deviation of both is half the blur. Tela's rectangles are closed
	// forms and its other shadows three boxes, as Skia's, whose boxes are of
	// whole pixels: one, which does nothing, for a blur of 1.
	whyBlur = "Gaussians against Skia's three boxes of whole pixels"
	// To the left, and under any transform; Tela's are at quarters of a pixel,
	// as other glyphs.
	whyEmoji = "where emoji are: at whole pixels in Skia"
	// See package text.
	whyOptical = "the optical size of the font, of whole pixels in Tela"
	// The advances of go-text are whole font units, the glyphs of both at
	// quarters of a pixel: a quarter away at most.
	whyPlace = "a glyph whose place rounds to another quarter of a pixel"
	// With a maxWidth Chromium scales the style with the text, from x = 0 (a
	// gradient from 10 to 150 starts at 7), and other browsers from the point
	// of the text. In Tela a style is in the canvas's coordinates, as for
	// everything else, as the HTML Standard says.
	whyCondensed = "what a condensed text is painted with, which Tela does not condense"
	whyClear     = "Chromium does not antialias clearRect; Tela does when it is not upright"
)

// sceneBounds are the bounds of the cells that chromiumBounds does not list,
// by scene.
var sceneBounds = map[string]bound{
	"paths":      {mean: 0.35, max: 60, why: whyEdges},
	"dashes":     {mean: 0.45, max: 30, why: whyEdges},
	"gradients":  {mean: 0.25, max: 25, why: whyRounding},
	"patterns":   {mean: 0.2, max: 5, why: whyRounding},
	"text":       {mean: 1.3, max: 75, why: whyText},
	"text_rtl":   {mean: 0.4, max: 85, why: whyText},
	"images":     {mean: 0.45, max: 5, why: whyRounding},
	"shadows":    {mean: 0.45, max: 30, why: whyBlur},
	"operations": {mean: 0.27, max: 55, why: whyEdges},
	"curves":     {mean: 0.5, max: 65, why: whyEdges},
	"lines":      {mean: 0.45, max: 60, why: whyEdges},
	"transforms": {mean: 0.3, max: 45, why: whyEdges},
	"gradients2": {mean: 0.26, max: 20, why: whyEdges},
	"patterns2":  {mean: 0.55, max: 5, why: whyRounding},
	"images2":    {mean: 0.4, max: 35, why: whyWeights + "; " + whyEdges},
	"clips":      {mean: 0.3, max: 60, why: whyEdges},
	"alpha":      {mean: 0.35, max: 35, why: whyEdges},
	"clear":      {mean: 0.1, max: 45, why: whyEdges},
	"nothing":    {mean: 0.1, max: 45, why: whyEdges},
	"sources":    {mean: 0.45, max: 60, why: whyEdges},
	"shadows2":   {mean: 0.45, max: 30, why: whyBlur + "; " + whyEdges},
	"shadow_ops": {mean: 0.4, max: 55, why: whyEdges},
	"baselines":  {mean: 0.33, max: 70, why: whyText},
	"text2":      {mean: 0.45, max: 70, why: whyText},
}

// chromiumBounds are the bounds of the cells that differ more than the others
// of their scene.
var chromiumBounds = map[string]bound{
	"paths/arcTo":   {mean: 0.25, max: 105, why: whyOverlap}, // its first line
	"paths/ellipse": {mean: 0.3, max: 80, why: whyEdges},
	"paths/Path2D":  {mean: 0.25, max: 75, why: whyEdges},

	"dashes/circle": {mean: 0.6, max: 175, why: whyDashes},

	"gradients/line":          {mean: 0.5, max: 130, why: whyEdges},
	"gradients/conic":         {mean: 0.4, max: 220, why: whySeam},
	"gradients/text":          {mean: 0.35, max: 65, why: whyText},
	"gradients/strokeText":    {mean: 0.7, max: 95, why: whyOverlap},
	"gradients/radial skewed": {mean: 0.25, max: 50, why: whyEdges},

	"patterns/shape":  {mean: 0.2, max: 65, why: whyEdges},
	"patterns/turned": {mean: 0.1, max: 35, why: whyEdges},
	"patterns/stroke": {mean: 0.85, max: 60, why: whyWeights + "; " + whyEdges},
	"patterns/text":   {mean: 0.15, max: 50, why: whyText},
	"patterns/scaled": {mean: 0.75, max: 15, why: whyWeights},

	"text/scripts":           {mean: 1.65, max: 220, why: whyEmoji},
	"text/turned":            {mean: 1.55, max: 190, why: whyEmoji},
	"text/faded":             {mean: 0.55, max: 80, why: whyEmoji},
	"text/maxWidth":          {mean: 1.6, max: 60, why: whyText},
	"text/strokeText":        {mean: 1.4, max: 155, why: whyOverlap},
	"text/dashed strokeText": {mean: 3.2, max: 190, why: whyDashes},
	"text/both":              {mean: 0.65, max: 125, why: whyOverlap},

	"images/stretched": {mean: 0.3, max: 15, why: whyWeights},
	"images/minified":  {mean: 0.3, max: 15, why: whyWeights},
	"images/turned":    {mean: 0.15, max: 25, why: whyEdges},
	"images/clipped":   {mean: 0.25, max: 40, why: whyEdges},

	"shadows/rounded":     {mean: 0.7, max: 50, why: whyBlur + "; " + whyEdges},
	"shadows/large blur":  {mean: 0.3, max: 35, why: whyBlur + "; " + whyEdges},
	"shadows/outside":     {mean: 0.15, max: 50, why: whyEdges},
	"shadows/text":        {mean: 0.3, max: 40, why: whyText},
	"shadows/emoji":       {mean: 0.25, max: 80, why: whyText},
	"shadows/gradient":    {mean: 0.75, max: 10, why: whyBlur},
	"shadows/globalAlpha": {mean: 0.5, max: 30, why: whyBlur},

	// Tela's is within 51 levels of the same ellipse made of 20000 lines in
	// Chromium, whose own is 177 from it.
	"curves/ellipse ratio":    {mean: 0.2, max: 175, why: whyEdges},
	"curves/ellipse flat":     {mean: 0.6, max: 120, why: whyOverlap},
	"curves/ellipse thin":     {mean: 0.55, max: 110, why: whyOverlap},
	"curves/quad sharp":       {mean: 0.15, max: 110, why: whyOverlap},
	"curves/roundRect dashes": {mean: 0.65, max: 170, why: whyDashes},

	"lines/wide circle":       {mean: 16.6, max: 185, kept: true, why: "a line wider than its circle: the HTML Standard covers its middle, where the inner outline of Skia's, turned inside out, leaves a hole"},
	"lines/dash dots square":  {mean: 1.8, max: 185, kept: true, why: "squares of dashes of no length: Skia's are upright, whatever the direction of the line"},
	"lines/dash longer":       {mean: 0.35, max: 185, kept: true, why: "a dash as long as a closed line: Skia's has no corner where it ends"},
	"lines/dash ellipse":      {mean: 0.65, max: 160, why: whyDashes},
	"lines/dash curve":        {mean: 0.45, max: 105, why: whyDashes},
	"lines/dash many":         {mean: 0.6, max: 100, why: whyEdges},
	"lines/curve joins":       {mean: 0.25, max: 95, why: whyEdges},
	"lines/curve round joins": {mean: 0.3, max: 95, why: whyEdges},

	"transforms/path kept through 0":  {mean: 24.5, max: 200, kept: true, why: "Blink takes a path through the wrong transforms around one that has no inverse"},
	"transforms/transform after path": {mean: 0.4, max: 85, why: whyEdges},

	"gradients2/conic hard": {mean: 0.2, max: 240, why: whySeam},

	"patterns2/text":              {mean: 0.2, max: 40, why: whyText},
	"patterns2/larger":            {mean: 2.1, max: 20, why: whyWeights},
	"patterns2/moved a bit":       {mean: 0.8, max: 15, why: whyWeights},
	"patterns2/smaller":           {mean: 0.95, max: 15, why: whyWeights},
	"patterns2/third":             {mean: 1, max: 20, why: whyWeights},
	"patterns2/photo third":       {mean: 1.15, max: 25, why: whyWeights},
	"patterns2/tenth":             {mean: 1.1, max: 15, why: whyWeights},
	"patterns2/under both":        {mean: 0.3, max: 10, why: whyWeights},
	"patterns2/under a transform": {mean: 0.15, max: 20, why: whyEdges},
	"patterns2/fine 0.7":          {mean: 3.8, max: 25, why: whyWeights},
	"patterns2/fine quarter":      {mean: 0.85, max: 1, why: whyRounding},
	"patterns2/fine tenth":        {mean: 1.2, max: 2, why: whyRounding},

	"images2/half, pixels":  {mean: 0.4, max: 255, kept: true, why: "pixels exactly between two of an image: the second one in Tela, at any scale"},
	"images2/turned region": {mean: 0.25, max: 65, why: whyEdges},

	"clips/copy":       {mean: 0.3, max: 75, why: whyEdges},
	"clips/same twice": {mean: 0.25, max: 65, why: whyEdges},

	"alpha/strokeText emoji": {mean: 0.75, max: 85, why: whyOverlap + "; " + whyText},
	"alpha/text and emoji":   {mean: 0.35, max: 55, why: whyText},

	"clear/turned": {mean: 0.45, max: 105, kept: true, why: whyClear},
	"clear/skewed": {mean: 0.4, max: 95, kept: true, why: whyClear},

	"sources/source-in strokeText":        {mean: 0.55, max: 125, why: whyOverlap},
	"sources/destination-in strokeText":   {mean: 0.35, max: 110, why: whyOverlap},
	"sources/destination-atop strokeText": {mean: 0.55, max: 110, why: whyOverlap},
	"sources/copy strokeText":             {mean: 0.7, max: 125, why: whyOverlap},
	"sources/multiply strokeText":         {mean: 0.75, max: 100, why: whyOverlap},
	"sources/multiply emoji":              {mean: 0.5, max: 60, why: whyText},
	"sources/xor strokeText":              {mean: 0.75, max: 230, kept: true, why: "Chromium composites a text glyph by glyph, and two that overlap erase each other, where the HTML Standard composites the text"},

	"shadows2/blur 1 only":         {mean: 0.95, max: 90, why: whyBlur},
	"shadows2/blur 1 circle only":  {mean: 0.45, max: 45, why: whyBlur},
	"shadows2/blur 1 image":        {mean: 1.2, max: 90, why: whyBlur},
	"shadows2/blur 1 rounded":      {mean: 0.4, max: 55, why: whyBlur + "; " + whyEdges},
	"shadows2/blur 1 text":         {mean: 1.15, max: 75, why: whyBlur + "; " + whyText},
	"shadows2/blur 3 text":         {mean: 0.9, max: 65, why: whyBlur + "; " + whyText},
	"shadows2/blur 8 text":         {mean: 0.65, max: 50, why: whyBlur + "; " + whyText},
	"shadows2/blur 20 text":        {mean: 1.5, max: 35, why: whyBlur + "; " + whyText},
	"shadows2/blur 8 rounded":      {mean: 0.5, max: 30, why: whyBlur + "; " + whyEdges},
	"shadows2/blur 20 rounded":     {mean: 0.7, max: 35, why: whyBlur + "; " + whyEdges},
	"shadows2/blur 3 circle only":  {mean: 0.55, max: 15, why: whyBlur},
	"shadows2/blur 8 circle only":  {mean: 0.8, max: 10, why: whyBlur},
	"shadows2/blur 20 circle only": {mean: 1.15, max: 10, why: whyBlur},
	"shadows2/blur 20 only":        {mean: 1.1, max: 10, why: whyBlur},
	"shadows2/blur 20":             {mean: 0.6, max: 10, why: whyBlur},
	"shadows2/blur 40":             {mean: 0.85, max: 10, why: whyBlur},
	"shadows2/blur 80":             {mean: 0.95, max: 10, why: whyBlur},
	"shadows2/strokeText":          {mean: 0.65, max: 55, why: whyOverlap},

	"shadow_ops/color-burn blur 8": {mean: 0.5, max: 55, why: whyEdges},

	"baselines/Source Serif 4 alphabetic":    {mean: 0.5, max: 95, why: whyOverlap},
	"baselines/Source Serif 4 top":           {mean: 0.5, max: 95, why: whyOverlap},
	"baselines/Source Serif 4 hanging":       {mean: 0.5, max: 95, why: whyOverlap},
	"baselines/Source Serif 4 middle":        {mean: 0.5, max: 95, why: whyOverlap},
	"baselines/Source Serif 4 ideographic":   {mean: 0.5, max: 95, why: whyOverlap},
	"baselines/Source Serif 4 bottom":        {mean: 0.5, max: 95, why: whyOverlap},
	"baselines/Noto Sans Arabic alphabetic":  {mean: 0.25, max: 75, why: whyOverlap},
	"baselines/Noto Sans Arabic top":         {mean: 0.25, max: 75, why: whyOverlap},
	"baselines/Noto Sans Arabic hanging":     {mean: 0.25, max: 75, why: whyOverlap},
	"baselines/Noto Sans Arabic ideographic": {mean: 0.25, max: 75, why: whyOverlap},
	"baselines/Noto Sans Arabic bottom":      {mean: 0.25, max: 75, why: whyOverlap},

	"text2/gradient condensed":        {mean: 4.7, max: 110, kept: true, why: whyCondensed},
	"text2/stroke gradient condensed": {mean: 9.7, max: 120, kept: true, why: whyCondensed},
	"text2/pattern condensed":         {mean: 8.7, max: 245, kept: true, why: whyCondensed},
	"text2/conic condensed":           {mean: 1.85, max: 90, kept: true, why: whyCondensed},

	"text2/spacing emoji":    {mean: 1.9, max: 220, kept: true, why: whyEmoji},
	"text2/emoji after text": {mean: 0.85, max: 220, kept: true, why: whyEmoji},
	"text2/emoji x+0.25":     {mean: 0.6, max: 175, kept: true, why: whyEmoji},
	"text2/emoji x+0.5":      {mean: 1.05, max: 155, kept: true, why: whyEmoji},
	"text2/emoji x+0.75":     {mean: 1.5, max: 215, kept: true, why: whyEmoji},
	"text2/emoji turned":     {mean: 1.45, max: 205, kept: true, why: whyEmoji},

	"text2/fraction size":           {mean: 0.45, max: 105, why: whyOptical},
	"text2/fraction place":          {mean: 0.25, max: 90, why: whyPlace + "; " + whyText},
	"text2/turned":                  {mean: 0.4, max: 85, why: whyPlace + "; " + whyText},
	"text2/large":                   {mean: 0.3, max: 75, why: whyText},
	"text2/round joins":             {mean: 0.3, max: 75, why: whyOverlap + "; " + whyEdges},
	"text2/spaced stroke left":      {mean: 0.3, max: 75, why: whyOverlap + "; " + whyEdges},
	"text2/spaced stroke center":    {mean: 0.4, max: 75, why: whyOverlap + "; " + whyEdges},
	"text2/spaced stroke right":     {mean: 0.5, max: 70, why: whyOverlap + "; " + whyEdges},
	"text2/stroke 0.5":              {mean: 0.65, max: 115, why: whyOverlap + "; " + whyEdges},
	"text2/stroke 1":                {mean: 0.9, max: 115, why: whyOverlap + "; " + whyEdges},
	"text2/stroke 2":                {mean: 0.95, max: 90, why: whyOverlap + "; " + whyEdges},
	"text2/stroke 6":                {mean: 0.4, max: 125, why: whyOverlap + "; " + whyEdges},
	"text2/stroke 10":               {mean: 0.35, max: 90, why: whyOverlap + "; " + whyEdges},
	"text2/stroke serif":            {mean: 1, max: 90, why: whyOverlap + "; " + whyEdges},
	"text2/stroke mono":             {mean: 0.55, max: 70, why: whyOverlap + "; " + whyEdges},
	"text2/stroke arabic":           {mean: 0.7, max: 115, why: whyOverlap + "; " + whyEdges},
	"text2/stroke overlaps":         {mean: 1.45, max: 90, why: whyOverlap + "; " + whyEdges},
	"text2/stroke gradient":         {mean: 0.65, max: 80, why: whyOverlap + "; " + whyEdges},
	"text2/condensed stroke left":   {mean: 0.5, max: 100, why: whyOverlap},
	"text2/condensed stroke center": {mean: 0.5, max: 100, why: whyOverlap},
	"text2/condensed stroke right":  {mean: 0.5, max: 100, why: whyOverlap},
}
