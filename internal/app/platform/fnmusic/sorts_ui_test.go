package fnmusic

import (
	"testing"

	backend "netease-music-mygo/internal/platform/fnmusic"

	"github.com/egoist/mygo/ui"
)

// Picking a sort field uses the page's default direction; the direction item
// changes only the current field.
func TestSortButtonFieldAndDirection(t *testing.T) {
	tests := []struct {
		name string
		item string
		want string
	}{
		{"field", "发行年份", backend.SortValue(backend.SortFieldReleaseYear, backend.SortOrderDesc)},
		{"direction", "升序 ↑", backend.SortValue(backend.SortFieldAlbumAddedAt, backend.SortOrderAsc)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			control := newSortControl(backend.AlbumListSortPolicy, albumSortFields, backend.AlbumListSortPolicy.Default, func(v string) { got = v })
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Padding(20).Children(func() { control.button(c, "test-sort") })
			}, 600, 300)
			tt.Frame()
			if !tt.HasText("排序 · 更新日期 ↓") {
				t.Fatal("the button did not show the default field and direction")
			}
			if err := tt.Click("排序"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(tc.item); err != nil {
				t.Fatalf("%v; the menu shows %q", err, tt.Menu())
			}
			if got != tc.want {
				t.Fatalf("%s chose %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
