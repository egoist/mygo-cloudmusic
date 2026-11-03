package fnmusic

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// With NM_FRAMES set, saves a sheet of style covers: every drawn scene, then
// generated ones for uncommon names, to look over.
func TestGenreArtSheet(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("NM_FRAMES is not set")
	}
	var names []string
	for _, th := range genreThemes {
		names = append(names, th.keys[0])
	}
	names = append(names, "资源来自：hrsony.com", "Shoegaze", "Vaporwave", "未命名风格", "Gothic", "Celtic", "Krautrock", "城市民谣", "Math", "Ἑλληνικά", "Zydeco", "Noise", "Dream", "Barbershop", "????")
	const cell, cols = 150, 7
	rows := (len(names) + cols - 1) / cols
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Grid(c).Columns(cols).Gap(10).Padding(10).Children(func() {
			for _, n := range names {
				ui.Column(c).Gap(4).Children(func() {
					genreArt(c, n, cell, 12)
					ui.Text(c, n).FontSize(11).SingleLine().Width(cell)
				})
			}
		})
	}, cols*(cell+10)+10, rows*(cell+30)+20)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "genre-art.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

// A style's name finds its drawn scene in English or Chinese, a narrower
// style wins over the one it is part of, and a short key must be a word:
// "post-rock" once came out a soundtrack, for the "ost" in it.
func TestGenreThemeMatching(t *testing.T) {
	for name, want := range map[string]string{
		"J-Pop": "jpop", "华语流行": "chinese", "Pop": "pop", "Synthpop": "electronic",
		"Post-Rock": "rock", "Hip-Hop/Rap": "hiphop", "Trap": "hiphop", "OST": "soundtrack",
		"爵士": "jazz", "R&B": "soul", "Heavy Metal": "metal", "古风": "chinese-traditional",
		"Grape": "", "Costume": "", "资源来自：hrsony.com": "",
	} {
		got := ""
		if th := genreThemeFor(name); th != nil {
			got = th.name
		}
		if got != want {
			t.Errorf("genreThemeFor(%q) = %q, want %q", name, got, want)
		}
	}
}

// A generated cover is the same for a name each time and differs between
// names, so no two uncommon styles share one.
func TestGeneratedGenreArtIsStableAndDistinct(t *testing.T) {
	pick := func(name string) [3]uint32 {
		seed := genreSeed(name)
		r := &genreRand{s: seed | 1}
		return [3]uint32{seed % uint32(len(genreLayouts)), r.next(), r.next()}
	}
	if pick("Shoegaze") != pick("  shoegaze ") {
		t.Fatal("the same style, written otherwise, got another cover")
	}
	seen := map[[3]uint32]string{}
	for _, n := range []string{"Shoegaze", "Vaporwave", "Gothic", "Celtic", "Krautrock", "Noise", "Dream", "Zydeco", "Math", "Barbershop"} {
		k := pick(n)
		if other, ok := seen[k]; ok {
			t.Fatalf("%q and %q got the same cover", n, other)
		}
		seen[k] = n
	}
}
