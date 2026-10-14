package core

import "testing"

// The space gallery: space-x-* and space-y-* in block and flex
// containers, negative values included (Tailwind's overlapping avatars,
// -space-x-2). Tailwind v4 implements them as margins on every child but
// the last, in both kinds of container, which this gallery compares with
// Chromium (cssGallery).

func spaceAvatars(n int, class string) []inode {
	colors := []string{"bg-rose-400", "bg-amber-400", "bg-emerald-400", "bg-sky-400", "bg-violet-400"}
	out := make([]inode, n)
	for i := range out {
		out[i] = iel("div", "size-10 shrink-0 rounded-full ring-2 ring-white "+colors[i%len(colors)]+" "+class)
	}
	return out
}

func spaceBoxes(n int, class string) []inode {
	out := make([]inode, n)
	for i := range out {
		out[i] = iel("div", "h-8 w-10 bg-indigo-"+[]string{"300", "500", "700"}[i%3]+" "+class)
	}
	return out
}

var spaceGallery = &cssGallery{
	name:  "space",
	width: 480,
	rows: []cssRow{
		{name: "avatars", height: 64, kids: []inode{
			iel("div", "flex -space-x-2", spaceAvatars(5, "")...),
			iel("div", "ml-10 flex -space-x-4", spaceAvatars(4, "")...),
		}},
		{name: "flex-positive", height: 48, kids: []inode{
			iel("div", "flex space-x-4", spaceBoxes(3, "")...),
			iel("div", "ml-8 flex space-x-1", spaceBoxes(3, "")...),
		}},
		{name: "flex-col", height: 136, class: "items-start", kids: []inode{
			iel("div", "flex flex-col space-y-2", spaceBoxes(3, "")...),
			iel("div", "ml-8 flex flex-col -space-y-3", spaceBoxes(3, "")...),
			iel("div", "ml-8 flex w-24 flex-col space-x-4", spaceBoxes(3, "w-full")...),
		}},
		{name: "block-y", height: 136, class: "items-start", kids: []inode{
			iel("div", "w-24 space-y-2", spaceBoxes(3, "w-24")...),
			iel("div", "ml-8 w-24 -space-y-4", spaceBoxes(3, "w-24")...),
			iel("div", "ml-8 w-24 -space-y-2", spaceBoxes(3, "w-24")...),
		}},
		{name: "wrap", height: 96, kids: []inode{
			iel("div", "flex w-[170px] flex-wrap space-x-4 space-y-2", spaceBoxes(5, "")...),
			iel("div", "ml-8 flex w-[170px] flex-wrap -space-x-3", spaceBoxes(5, "")...),
		}},
		{name: "justify", height: 48, kids: []inode{
			iel("div", "flex w-[200px] justify-between space-x-4 bg-slate-100", spaceBoxes(3, "")...),
			iel("div", "ml-8 flex w-[200px] justify-end -space-x-3 bg-slate-100", spaceBoxes(3, "")...),
		}},
		{name: "own-margin", height: 48, kids: []inode{
			iel("div", "flex space-x-4", spaceBoxes(3, "mr-2")...),
			iel("div", "ml-8 flex -space-x-3", spaceBoxes(3, "ml-3")...),
			iel("div", "ml-8 flex space-x-4", spaceBoxes(3, "mx-1")...),
		}},
		{name: "hidden-last", height: 48, kids: []inode{
			iel("div", "flex space-x-4", append(spaceBoxes(2, ""), iel("div", "hidden h-8 w-10 bg-rose-500"))...),
			iel("div", "ml-8 flex space-x-4", append(spaceBoxes(2, ""), iel("div", "absolute h-8 w-10 bg-rose-500"))...),
		}},
		{name: "inline", height: 48, kids: []inode{
			iel("div", "inline-flex -space-x-2", spaceAvatars(3, "")...),
			iel("div", "ml-8 inline-flex space-x-3", spaceBoxes(3, "")...),
			iel("div", "ml-8 flex items-center gap-2 -space-x-2", spaceAvatars(3, "")...),
		}},
	},
	mean: [2]float64{0.3, 0.3},
	geom: [2]float64{0.02, 0.02},
}

func TestChromiumSpace(t *testing.T) { spaceGallery.compare(t) }

func TestSpaceGallery(t *testing.T) { spaceGallery.golden(t) }
