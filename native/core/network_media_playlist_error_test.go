package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// 回归测试：播放列表读取错误必须区分为「取消 / 读取失败 / 过大」三类，
// 不能把切集导致的 context.Canceled 误报成「播放列表过大或读取失败」。
func TestReadMediaPlaylistBodyClassifiesErrors(t *testing.T) {
	t.Run("正常播放列表", func(t *testing.T) {
		playlist := "#EXTM3U\n#EXT-X-ENDLIST\n"
		body, err := readMediaPlaylistBody(context.Background(), strings.NewReader(playlist))
		if err != nil {
			t.Fatalf("正常读取不应报错: %v", err)
		}
		if string(body) != playlist {
			t.Fatalf("内容不一致: got %q", string(body))
		}
	})

	t.Run("切集取消应返回 context.Canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 模拟用户切集/退出，context 已取消
		_, err := readMediaPlaylistBody(ctx, strings.NewReader("#EXTM3U\n"))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("期望 context.Canceled，实际: %v", err)
		}
		if errors.Is(err, io.EOF) {
			t.Fatalf("不应是 EOF: %v", err)
		}
	})

	t.Run("超过上限报过大且不含读取失败字样", func(t *testing.T) {
		// 用有限长度的 zero reader 产出 max+2 字节，确保触发过大判断。
		big := &zeroReader{remaining: mediaPlaylistMaxBytes + 2}
		_, err := readMediaPlaylistBody(context.Background(), big)
		if err == nil {
			t.Fatal("超过上限应报错")
		}
		if !strings.Contains(err.Error(), "过大") {
			t.Fatalf("错误信息应包含『过大』: %v", err)
		}
		if strings.Contains(err.Error(), "读取失败") {
			t.Fatalf("过大不应误报为『读取失败』: %v", err)
		}
	})

	t.Run("底层读取错误归类为读取失败并保留原因", func(t *testing.T) {
		sentinel := errors.New("connection reset by peer")
		_, err := readMediaPlaylistBody(context.Background(), &errReader{err: sentinel})
		if err == nil {
			t.Fatal("读取错误应返回")
		}
		if !strings.Contains(err.Error(), "读取失败") {
			t.Fatalf("错误信息应包含『读取失败』: %v", err)
		}
		if !errors.Is(err, sentinel) {
			t.Fatalf("应保留底层错误链: %v", err)
		}
	})

	t.Run("包装后 context.Canceled 仍可被 errors.Is 识别", func(t *testing.T) {
		_, err := readMediaPlaylistBody(canceledContext(), strings.NewReader("#EXTM3U\n"))
		wrapped := fmt.Errorf("获取播放列表失败: %w", err)
		if !errors.Is(wrapped, context.Canceled) {
			t.Fatalf("上层包装后应仍识别取消: %v", wrapped)
		}
	})
}

// errReader 模拟底层读取失败。
type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

// zeroReader 产出指定数量的 0 字节，随后 EOF，用于测试体积上限。
type zeroReader struct{ remaining int }

func (r *zeroReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 0
	}
	r.remaining -= n
	return n, nil
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
