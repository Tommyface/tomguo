package core

import (
	"testing"
	"time"
)

func TestInspectPlaylist(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantDur   time.Duration
		wantSegs  int
		wantValid bool
	}{
		{
			name: "正常多分片列表",
			raw: "#EXTM3U\n#EXT-X-VERSION:3\n" +
				"#EXTINF:10.0,\nseg0.ts\n" +
				"#EXTINF:10.0,\nseg1.ts\n" +
				"#EXTINF:10.0,\nseg2.ts\n",
			wantDur:   30 * time.Second,
			wantSegs:  3,
			wantValid: true,
		},
		{
			name:      "带 BOM 的列表",
			raw:       "\ufeff#EXTM3U\n#EXTINF:20.0,\nseg0.ts\n",
			wantDur:   20 * time.Second,
			wantSegs:  1,
			wantValid: true,
		},
		{
			name:      "非法列表缺少 EXTM3U",
			raw:       "#EXTINF:5.0,\nseg0.ts\n",
			wantDur:   5 * time.Second,
			wantSegs:  1,
			wantValid: false,
		},
		{
			name:      "空列表",
			raw:       "",
			wantDur:   0,
			wantSegs:  0,
			wantValid: false,
		},
		{
			name: "含非法时长行应跳过",
			raw: "#EXTM3U\n#EXTINF:abc,\nbad.ts\n" +
				"#EXTINF:-1,\nneg.ts\n" +
				"#EXTINF:12.5,\ngood.ts\n",
			wantDur:   12500 * time.Millisecond,
			wantSegs:  1,
			wantValid: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inspectPlaylist(tc.raw)
			if got.Duration != tc.wantDur {
				t.Errorf("Duration = %s, want %s", got.Duration, tc.wantDur)
			}
			if got.Segments != tc.wantSegs {
				t.Errorf("Segments = %d, want %d", got.Segments, tc.wantSegs)
			}
			if got.ValidM3U8 != tc.wantValid {
				t.Errorf("ValidM3U8 = %v, want %v", got.ValidM3U8, tc.wantValid)
			}
		})
	}
}

func TestLooksLikePreviewFragment(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "正常剧集不应误报",
			raw: "#EXTM3U\n" +
				"#EXTINF:30.0,\nseg0.ts\n" +
				"#EXTINF:30.0,\nseg1.ts\n",
			want: false,
		},
		{
			name: "极短单分片占位片段应命中",
			raw:  "#EXTM3U\n#EXTINF:3.0,\npreview.ts\n",
			want: true,
		},
		{
			name: "阈值边界内应命中(14秒)",
			raw:  "#EXTM3U\n#EXTINF:14.0,\npreview.ts\n",
			want: true,
		},
		{
			name: "阈值边界外不命中(16秒)",
			raw:  "#EXTM3U\n#EXTINF:16.0,\nseg.ts\n",
			want: false,
		},
		{
			name: "非法列表不命中",
			raw:  "#EXTINF:3.0,\npreview.ts\n",
			want: false,
		},
		{
			name: "空列表不命中",
			raw:  "",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikePreviewFragment(inspectPlaylist(tc.raw))
			if got != tc.want {
				t.Errorf("looksLikePreviewFragment = %v, want %v (stats=%+v)", got, tc.want, inspectPlaylist(tc.raw))
			}
		})
	}
}

func TestM3u8DurationBackwardCompatible(t *testing.T) {
	raw := "#EXTM3U\n#EXTINF:9.5,\na.ts\n#EXTINF:10.5,\nb.ts\n"
	if got := m3u8Duration(raw); got != 20*time.Second {
		t.Errorf("m3u8Duration = %s, want 20s", got)
	}
}
