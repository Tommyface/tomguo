package core

// custom_aes.go
//
// 参数化 AES 站源引擎（运行时配置驱动）。
//
// 覆盖形态（如实声明）：
//   - 请求：POST form-urlencoded，可配签名拼接模板（占位符见下）
//   - 响应：AES-CBC 解密，key/iv 支持明文 UTF-8 或 hex，PKCS7
//   - 业务数据：解密后为 JSON，用启发式字段匹配提取剧集/详情/播放地址
//
// 这是「算法同构」的通用解。以下非标形态**不在覆盖范围**，仍需专属 Go 代码
// （即仍需构建）：动态 key 协商（如橙果的 visitors/session + 双密钥）、
// 请求体 gzip 压缩、自定义二进制协议、非标准签名算法。
//
// 签名模板支持的占位符：
//   {path} {timestamp} {signKey} {query} —— 依序拼接后先 SHA256 再 MD5
//   （与项目内黄豆/橙果签名风格一致；留空则不签名）

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const customAESPlayScheme = "custom-aes-play://"

// aesRequest 发送一次参数化 AES 请求并解密出业务 JSON。
func (d *Downloader) aesRequest(ctx context.Context, cfg customSourceConfig, path string, params map[string]string) (map[string]any, error) {
	apiBase := strings.TrimRight(firstNonEmpty(cfg.APIURL, cfg.BaseURL), "/")
	fullPath := "/" + strings.TrimLeft(path, "/")
	endpoint := apiBase + fullPath

	if params == nil {
		params = map[string]string{}
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	if cfg.SignTemplate != "" {
		signed := signTemplate(cfg.SignTemplate, fullPath, timestamp, cfg.SignKey, params)
		sum := sha256.Sum256([]byte(signed))
		params["sign"] = fmt.Sprintf("%x", md5.Sum([]byte(hex.EncodeToString(sum[:]))))
		params["timestamp"] = timestamp
	}

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", cfg.BaseURL+"/")
	req.Header.Set("Origin", cfg.BaseURL)

	resp, err := d.doPreparedCatalogRequest(req, 20*time.Second, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	blob, err := io.ReadAll(io.LimitReader(resp.Body, providerMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(blob) > providerMaxBodyBytes {
		return nil, errors.New("响应过大")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 先按明文 JSON 试解；若 data 字段是密文则解密。
	var envelope map[string]any
	if err := json.Unmarshal(blob, &envelope); err != nil {
		// 整个响应可能就是一段密文（Base64）。
		decoded, derr := aesDecryptPayload(blob, cfg)
		if derr != nil {
			return nil, fmt.Errorf("响应既非 JSON 也无法解密: %w", derr)
		}
		blob = decoded
		if err := json.Unmarshal(blob, &envelope); err != nil {
			return nil, fmt.Errorf("解密后不是 JSON: %w", err)
		}
		return envelope, nil
	}
	if data, ok := envelope["data"].(string); ok && data != "" {
		decoded, derr := aesDecryptPayload([]byte(data), cfg)
		if derr == nil {
			var inner map[string]any
			if json.Unmarshal(decoded, &inner) == nil {
				return inner, nil
			}
			// 解密后是数组：包一层。
			var arr []any
			if json.Unmarshal(decoded, &arr) == nil {
				return map[string]any{"list": arr}, nil
			}
		}
	}
	return envelope, nil
}

func aesDecryptPayload(blob []byte, cfg customSourceConfig) ([]byte, error) {
	key, err := decodeAESKey(cfg.AESKey, cfg.KeyIsHex)
	if err != nil {
		return nil, err
	}
	iv, err := decodeAESKey(cfg.AESIV, cfg.KeyIsHex)
	if err != nil {
		return nil, err
	}
	if len(iv) == 0 {
		iv = make([]byte, aes.BlockSize)
	}
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("AES 密钥长度无效: %d", len(key))
	}
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("AES IV 长度无效: %d", len(iv))
	}
	// 密文常见两种编码：Base64 文本、或原始字节（首 16 字节为 IV）。
	text := strings.TrimSpace(string(blob))
	cipherText, decErr := base64.StdEncoding.DecodeString(strings.ReplaceAll(text, " ", "+"))
	if decErr != nil {
		// 原始字节：若前 16 字节像 IV 则剥掉。
		cipherText = bytes.TrimSpace(blob)
		if len(cipherText) > aes.BlockSize*2 {
			iv = cipherText[:aes.BlockSize]
			cipherText = cipherText[aes.BlockSize:]
		}
	}
	return aesCBCDecrypt(cipherText, key, iv)
}

func decodeAESKey(raw string, isHex bool) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if isHex {
		return hex.DecodeString(strings.TrimPrefix(raw, "0x"))
	}
	return []byte(raw), nil
}

func signTemplate(tmpl, path, timestamp, signKey string, params map[string]string) string {
	s := strings.ReplaceAll(tmpl, "{path}", path)
	s = strings.ReplaceAll(s, "{timestamp}", timestamp)
	s = strings.ReplaceAll(s, "{signKey}", signKey)
	s = strings.ReplaceAll(s, "{query}", url.QueryEscape(params["query"]))
	// 支持 {param:xxx} 引用任意参数。
	for k, v := range params {
		s = strings.ReplaceAll(s, "{param:"+k+"}", v)
	}
	return s
}

// ---- 启发式 JSON 提取：覆盖常见字段命名 ----

func findList(data map[string]any) []any {
	for _, key := range []string{"list", "dramas", "items", "data", "rows", "videos", "records", "result", "content"} {
		if arr, ok := asSlice(data[key]); ok && len(arr) > 0 {
			// 若 arr 里是详情对象而非剧集，也照常返回（后续按字段提取）。
			return arr
		}
	}
	// 嵌套一层：{"data":{"list":[...]}}
	if inner, ok := data["data"].(map[string]any); ok {
		return findList(inner)
	}
	return nil
}

func asSlice(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case map[string]any:
		if arr, ok := x["list"].([]any); ok {
			return arr, true
		}
	}
	return nil, false
}

func dramaFromMap(item map[string]any, source string) Drama {
	id := firstNonEmpty(
		mapString(item, "id"), mapString(item, "drama_id"), mapString(item, "vid"),
		mapString(item, "video_id"), mapString(item, "playlet_id"), mapString(item, "slug"),
	)
	title := firstNonEmpty(mapString(item, "title"), mapString(item, "name"), mapString(item, "video_name"))
	if id == "" || title == "" {
		return Drama{}
	}
	cover := firstNonEmpty(
		mapString(item, "cover"), mapString(item, "cover_url"), mapString(item, "pic"),
		mapString(item, "img"), mapString(item, "poster"), mapString(item, "thumbnail"),
	)
	eps := firstNonEmpty(mapString(item, "episodes"), mapString(item, "total_episodes"),
		mapString(item, "chapter_count"), mapString(item, "total"), mapString(item, "num"))
	n, _ := strconv.Atoi(eps)
	return Drama{
		ID:       source + ":" + id,
		Source:   source,
		SourceID: id,
		Title:    title,
		Cover:    cover,
		Episodes: n,
		Desc:     firstNonEmpty(mapString(item, "desc"), mapString(item, "intro"), mapString(item, "description")),
	}
}

// ---- 目录 / 详情 / 播放接入 ----

func (d *Downloader) fetchCustomAESCatalogPage(ctx context.Context, cfg customSourceConfig, page int, category, query string) ([]Drama, bool, error) {
	params := map[string]string{"page": strconv.Itoa(page)}
	if category != "" {
		params["category"] = category
	}
	if query != "" {
		params["query"] = query
		params["q"] = query
		params["keyword"] = query
	}
	data, err := d.aesRequest(ctx, cfg, cfg.CatalogPath, params)
	if err != nil {
		return nil, false, err
	}
	items := findList(data)
	out := make([]Drama, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if drama := dramaFromMap(m, cfg.ID); drama.ID != "" {
			out = append(out, drama)
		}
	}
	return out, len(items) >= 20, nil
}

func (d *Downloader) fetchCustomAESDetail(ctx context.Context, cfg customSourceConfig, sourceID string) (Drama, []Chapter, error) {
	data, err := d.aesRequest(ctx, cfg, cfg.DetailEP, map[string]string{"id": sourceID})
	if err != nil {
		return Drama{}, nil, err
	}
	// 详情对象可能在根节点，也可能在 data 下。
	detail := data
	if inner, ok := data["data"].(map[string]any); ok {
		detail = inner
	}
	drama := dramaFromMap(detail, cfg.ID)
	drama.ID = cfg.ID + ":" + sourceID
	drama.SourceID = sourceID

	// 分集：优先 episodes 数组，其次 playlist。
	var episodeList []any
	if arr, ok := detail["episodes"].([]any); ok {
		episodeList = arr
	} else if arr, ok := detail["playlist"].([]any); ok {
		episodeList = arr
	} else if arr, ok := data["episodes"].([]any); ok {
		episodeList = arr
	}
	chapters := make([]Chapter, 0, len(episodeList))
	for i, raw := range episodeList {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		num := firstNonEmpty(mapString(m, "number"), mapString(m, "episode"), mapString(m, "n"), strconv.Itoa(i+1))
		eid := firstNonEmpty(mapString(m, "id"), num)
		media := firstNonEmpty(mapString(m, "url"), mapString(m, "m3u8"), mapString(m, "video_url"), mapString(m, "play_url"))
		if media == "" {
			// 没有直链：用占位 scheme，播放时按 id 再请求播放接口。
			media = customAESPlayScheme + cfg.ID + "/" + sourceID + "/" + eid
		}
		chapters = append(chapters, Chapter{
			ID:             cfg.ID + ":" + sourceID + ":" + eid,
			Source:         cfg.ID,
			Title:          "第" + num + "集",
			VideoURL:       media,
			CurrentEpisode: json.RawMessage(strconv.Quote(num)),
			Referer:        cfg.BaseURL + "/",
		})
	}
	if len(chapters) == 0 {
		return Drama{}, nil, errors.New("未解析到分集")
	}
	return drama, chapters, nil
}

func (d *Downloader) resolveCustomAESPlayback(ctx context.Context, task Task) (providerMedia, error) {
	// custom-aes-play://<source>/<dramaID>/<episodeID>
	rest := strings.TrimPrefix(task.Chapter.VideoURL, customAESPlayScheme)
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return providerMedia{}, errors.New("播放地址格式无效")
	}
	source, dramaID, episodeID := parts[0], parts[1], parts[2]
	if d.customSources == nil {
		return providerMedia{}, errors.New("自定义站源配置未加载")
	}
	cfg, ok := d.customSources.get(source)
	if !ok || cfg.Type != "aes" {
		return providerMedia{}, errors.New("自定义站源配置已失效")
	}
	data, err := d.aesRequest(ctx, cfg, cfg.PlayPath, map[string]string{
		"id": dramaID, "episode": episodeID, "ep": episodeID,
	})
	if err != nil {
		return providerMedia{}, err
	}
	media := findMediaURL(data)
	if media == "" {
		return providerMedia{}, errors.New("未解析到播放地址，该站可能需要特定参数或会员")
	}
	media = resolveProviderURL(cfg.BaseURL+"/", media)
	return providerMedia{URL: media, Referer: cfg.BaseURL + "/"}, nil
}

func findMediaURL(data map[string]any) string {
	for _, key := range []string{"url", "m3u8", "video_url", "play_url", "media", "playurl", "video"} {
		if v := mapString(data, key); v != "" {
			return v
		}
	}
	if inner, ok := data["data"].(map[string]any); ok {
		return findMediaURL(inner)
	}
	if arr, ok := data["urls"].([]any); ok && len(arr) > 0 {
		if first, ok := arr[0].(map[string]any); ok {
			return mapString(first, "url", "m3u8")
		}
		if s, ok := arr[0].(string); ok {
			return s
		}
	}
	return ""
}
