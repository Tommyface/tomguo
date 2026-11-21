package core

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 回归测试：加密 HLS（AES-128，session.key 非空）的分片必须作为二进制透传，
// 不能被误判为 m3u8 playlist 解析——否则返回 502「播放列表无效」，ExoPlayer 报
// Source error。黄豆等加密源全集播放失败即由此引起。
func TestNativeServeEncryptedHLSSegmentsPassThrough(t *testing.T) {
	// 上游：分片返回 TS 二进制，key 返回 16 字节。
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".ts"):
			w.Header().Set("Content-Type", "video/mp2t")
			// TS 包以 0x47 sync byte 开头，绝不可能被当作 #EXTM3U。
			_, _ = w.Write([]byte{0x47, 0x40, 0x00, 0x10, 0x00, 0x00, 0x00, 0x01})
		case strings.HasSuffix(r.URL.Path, ".key"):
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(make([]byte, 16))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	engine, err := newNativeEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.downloads.close)

	key := []byte("0123456789abcdef") // 16 字节，触发加密 HLS 路径
	playlist := fmt.Sprintf(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-KEY:METHOD=AES-128,URI="%s/enc.key",IV=0x00000000000000000000000000000000
#EXTINF:10.0,
%s/seg_000.ts
#EXT-X-ENDLIST
`, upstream.URL, upstream.URL)
	media := providerMedia{
		URL:      upstream.URL + "/playlist.m3u8",
		Referer:  upstream.URL + "/",
		Playlist: playlist,
		HLSKey:   key,
	}

	stream, err := newNativeStreamServer(engine.downloader)
	if err != nil {
		t.Fatal(err)
	}
	addr, _ := stream.nativeOpen(media)

	client := &http.Client{}
	// 1) 主 m3u8 应返回重写后的播放列表
	resp, err := client.Get(addr)
	if err != nil {
		t.Fatalf("GET 本地 m3u8 失败: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "#EXTM3U") {
		t.Fatalf("主 m3u8 返回异常: status=%d body=%q", resp.StatusCode, string(body))
	}

	// 2) 从重写结果里取出分片的本地地址
	var segURL string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http://127.0.0.1") && strings.HasSuffix(line, ".ts") {
			segURL = line
			break
		}
	}
	if segURL == "" {
		t.Fatalf("重写后的 m3u8 未找到分片地址:\n%s", string(body))
	}

	// 3) 分片必须是二进制透传（200 + video/mp2t），而非 502「播放列表无效」
	segResp, err := client.Get(segURL)
	if err != nil {
		t.Fatalf("GET 本地分片失败: %v", err)
	}
	segBody, _ := io.ReadAll(segResp.Body)
	segResp.Body.Close()
	if segResp.StatusCode != http.StatusOK {
		t.Fatalf("分片被误判: status=%d body=%q（期望 200 二进制透传）", segResp.StatusCode, string(segBody))
	}
	if strings.Contains(segResp.Header.Get("Content-Type"), "mpegurl") {
		t.Fatalf("分片 Content-Type 错误: %q", segResp.Header.Get("Content-Type"))
	}
	if len(segBody) == 0 || segBody[0] != 0x47 {
		t.Fatalf("分片内容不是透传的 TS 二进制: %v", segBody)
	}
}
