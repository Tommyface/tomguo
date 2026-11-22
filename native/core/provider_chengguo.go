package core

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
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
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	chengguoBaseURL   = "https://chengguodj.com"
	chengguoMirrorURL = "https://12.cyjoifcwr.cc"
	chengguoAPIURL    = "https://api2.chengguodj.com"

	// 橙果上游 API 的加密参数（AES-128-CBC / PKCS7 + SHA256→MD5 签名）。
	// 这些是上游公开前端 JS 内置的默认值（任何访问站点者均可从页面脚本获得），
	// 并非私有凭证；此前留空导致 aes.NewCipher 直接报 "invalid key size 0"，
	// 请求在构造阶段即失败（HTTP 0 / 0ms），内置橙果源完全不可用。
	// 上游通过 NUXT_PUBLIC_API_CRYPT_* 环境变量可覆盖这些默认值；若上游更换密钥，
	// 同步更新此处即可（与 provider_yeguo 的公开默认值回退策略一致）。
	chengguoAESKey  = "2acf7e91e9864673"
	chengguoAESIV   = "1c29882d3ddfcfd6"
	chengguoSignKey = "5589d41f92a597d016b037ac37db243d"
	chengguoVersion = "v0"
	chengguoClient  = "ios"

	chengguoPlayScheme = "chengguo-play://"

	chengguoAPIMirror1 = "https://api3.chengguodj.com"
	chengguoAPIMirror2 = "https://api1.chengguodj.com"
)

func validChengguoID(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "/\\:?#%|\"<>") &&
		!strings.HasPrefix(value, "@")
}

func chengguoPad(plain []byte) []byte {
	missing := aes.BlockSize - len(plain)%aes.BlockSize
	return append(plain, bytes.Repeat([]byte{byte(missing)}, missing)...)
}

func chengguoEncrypt(plain []byte) (string, error) {
	block, err := aes.NewCipher([]byte(chengguoAESKey))
	if err != nil {
		return "", err
	}
	padded := chengguoPad(plain)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, []byte(chengguoAESIV)).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

func chengguoDecrypt(text string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(text), " ", "+"))
	if err != nil || len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return nil, errors.New("橙果响应密文格式无效")
	}
	block, err := aes.NewCipher([]byte(chengguoAESKey))
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, []byte(chengguoAESIV)).CryptBlocks(out, raw)
	last := int(out[len(out)-1])
	if last < 1 || last > aes.BlockSize || len(out) < last {
		return nil, errors.New("橙果响应填充无效")
	}
	for _, pad := range out[len(out)-last:] {
		if int(pad) != last {
			return nil, errors.New("橙果响应填充无效")
		}
	}
	return out[:len(out)-last], nil
}

func chengguoSignature(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key == "sign" || key == "_ver" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := params[key]
		if key == "data" {
			value = strings.ReplaceAll(value, " ", "+")
		}
		parts = append(parts, key+"="+value)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "&") + chengguoSignKey))
	digest := md5.Sum([]byte(hex.EncodeToString(sum[:])))
	return hex.EncodeToString(digest[:])
}

func (d *Downloader) chengguoAPIHosts() []string {
	if configured := strings.TrimRight(strings.TrimSpace(d.cfg.ChengguoAPIURL), "/"); configured != "" {
		return []string{configured}
	}
	hosts := []string{chengguoAPIURL}
	for _, host := range []string{chengguoAPIMirror1, chengguoAPIMirror2} {
		if host != chengguoAPIURL {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func (d *Downloader) chengguoPost(ctx context.Context, route string, body map[string]any, jsonBody bool) (map[string]any, error) {
	return d.chengguoPostHosts(ctx, d.chengguoAPIHosts(), route, body, jsonBody)
}

func (d *Downloader) chengguoPostHosts(ctx context.Context, hosts []string, route string, body map[string]any, jsonBody bool) (map[string]any, error) {
	failures := make([]string, 0, len(hosts))
	for index, host := range hosts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		decoded, retryable, err := d.chengguoPostOnce(ctx, host+route, body, jsonBody)
		if err == nil {
			return decoded, nil
		}
		if !retryable {
			return nil, err
		}
		failures = append(failures, fmt.Sprintf("%s：%v", host, err))
		if index+1 < len(hosts) {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("橙果连接失败（已尝试 %d 个接口域名）：%s", len(failures), strings.Join(failures, "；"))
}

func (d *Downloader) chengguoPostOnce(ctx context.Context, endpoint string, body map[string]any, jsonBody bool) (map[string]any, bool, error) {
	site := d.providerBaseURL(sourceChengguo)
	var payload io.Reader
	contentType := "application/x-www-form-urlencoded"
	if jsonBody {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, false, errors.New("橙果请求参数无效")
		}
		payload = bytes.NewReader(encoded)
		contentType = "application/json"
	} else {
		plain, err := json.Marshal(body)
		if err != nil {
			return nil, false, errors.New("橙果请求参数无效")
		}
		cipherText, err := chengguoEncrypt(plain)
		if err != nil {
			return nil, false, errors.New("橙果请求加密失败")
		}
		params := map[string]string{
			"_ver":      chengguoVersion,
			"client":    chengguoClient,
			"data":      cipherText,
			"timestamp": strconv.FormatInt(time.Now().Unix(), 10),
		}
		params["sign"] = chengguoSignature(params)
		form := url.Values{}
		for _, key := range []string{"_ver", "client", "data", "sign", "timestamp"} {
			form.Set(key, params[key])
		}
		payload = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, false, errors.New("无法创建橙果请求")
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	request.Header.Set("Referer", site+"/")
	request.Header.Set("Origin", site)
	response, err := d.doPreparedCatalogRequest(request, 15*time.Second, nil)
	if err != nil {
		return nil, true, fmt.Errorf("%w", publicError(err))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			fmt.Errorf("橙果接口返回 HTTP %d", response.StatusCode)
	}
	blob, err := io.ReadAll(io.LimitReader(response.Body, providerMaxBodyBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("橙果响应读取失败：%w", publicError(err))
	}
	if int64(len(blob)) > providerMaxBodyBytes {
		return nil, false, errors.New("橙果响应超过大小限制")
	}
	var envelope map[string]any
	if err := json.Unmarshal(blob, &envelope); err != nil || mapString(envelope, "data") == "" {
		return nil, false, errors.New("橙果返回的数据格式无效")
	}
	signParams := map[string]string{}
	for key, value := range envelope {
		if key == "sign" || key == "_ver" || value == nil {
			continue
		}
		text := ""
		switch value := value.(type) {
		case string:
			text = value
		case json.Number:
			text = value.String()
		case bool:
			text = strconv.FormatBool(value)
		default:
			encoded, err := json.Marshal(value)
			if err != nil {
				continue
			}
			text = string(encoded)
		}
		signParams[key] = text
	}
	if signature := mapString(envelope, "sign"); signature != "" && !strings.EqualFold(signature, chengguoSignature(signParams)) {
		return nil, false, errors.New("橙果响应签名校验失败")
	}
	plain, err := chengguoDecrypt(signParams["data"])
	if err != nil {
		return nil, false, err
	}
	var decoded struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
		Error   map[string]any `json:"error"`
	}
	if err := json.Unmarshal(plain, &decoded); err != nil {
		return nil, false, errors.New("橙果返回的数据格式无效")
	}
	if !decoded.Success || decoded.Data == nil {
		if message := mapString(decoded.Error, "message"); message != "" {
			return nil, false, errors.New(message)
		}
		return nil, false, errors.New("橙果接口未返回有效内容")
	}
	return decoded.Data, false, nil
}

func chengguoCover(row map[string]any, site string) string {
	var candidate string
	switch cover := row["cover"].(type) {
	case string:
		candidate = strings.TrimSpace(cover)
	case map[string]any:
		if cover["encrypted"] == true {
			candidate = mapString(cover, "fallback_url", "url")
		} else {
			candidate = mapString(cover, "url", "fallback_url")
		}
	}
	if candidate == "" {
		candidate = mapString(row, "cover_url")
	}
	if strings.HasPrefix(candidate, "/") {
		candidate = site + candidate
	}
	return providerCoverAddress(candidate, site+"/")
}

func chengguoTags(row map[string]any) []string {
	var tags []string
	seen := map[string]bool{}
	switch entries := row["tags"].(type) {
	case []any:
		for _, entry := range entries {
			name := ""
			switch value := entry.(type) {
			case map[string]any:
				name = mapString(value, "name")
			case string:
				name = value
			}
			name = strings.TrimSpace(name)
			if name != "" && len([]rune(name)) <= 32 && !seen[name] && len(tags) < 24 {
				seen[name] = true
				tags = append(tags, name)
			}
		}
	}
	return tags
}

func chengguoStatus(row map[string]any) string {
	status := mapString(row, "serial_status", "status")
	switch status {
	case "completed", "finished":
		return "finished"
	case "ongoing":
		return "ongoing"
	}
	return ""
}

func chengguoTotalEpisodes(row map[string]any) int {
	for _, key := range []string{"total_episode_count", "total_episodes", "latest_episode_number"} {
		if value, ok := huangjuInteger(row[key], 100000); ok && value > 0 {
			return value
		}
	}
	return 0
}

func chengguoDramaFromRow(row map[string]any, site string) (Drama, error) {
	sourceID := strings.TrimSpace(mapString(row, "slug"))
	title := strings.TrimSpace(mapString(row, "title"))
	if !validChengguoID(sourceID) || title == "" {
		return Drama{}, errors.New("橙果返回的剧集信息不完整")
	}
	total := chengguoTotalEpisodes(row)
	description := mapString(row, "intro", "description", "alias")
	heat := mapString(row, "heat_7d", "real_heat_7d")
	tags := chengguoTags(row)
	category := "短剧"
	if len(tags) > 0 {
		category = tags[0]
	}
	cover := chengguoCover(row, site)
	return Drama{
		ID: providerDramaID(sourceChengguo, sourceID), Source: sourceChengguo, SourceID: sourceID,
		Title: truncate(title, 256), Name: truncate(title, 256),
		Desc: truncate(description, 12000), Intro: truncate(description, 12000),
		Cover: cover, CoverURL: cover, TotalEpisode: total, EpisodeCount: total,
		Category: category, ChannelName: "橙果", Tags: tags,
		ReleaseStatus: chengguoStatus(row), Heat: heat,
	}, nil
}

func (d *Downloader) fetchChengguoCategories(ctx context.Context) ([]nativeCategory, error) {
	data, err := d.chengguoPost(ctx, "/api/v1/tags?page=1&page_size=50", map[string]any{}, false)
	if err != nil {
		return nil, err
	}
	rows, valid := data["items"].([]any)
	if !valid || len(rows) > 300 {
		return nil, errors.New("橙果分类数据格式无效")
	}
	var categories []nativeCategory
	seen := map[string]bool{}
	for _, entry := range rows {
		row, _ := entry.(map[string]any)
		id, name := mapString(row, "slug"), mapString(row, "name")
		if !validChengguoID(id) || id == "" || name == "" || len([]rune(name)) > 48 || seen[id] {
			continue
		}
		seen[id] = true
		categories = append(categories, nativeCategory{ID: id, Name: name})
	}
	return categories, nil
}

func (d *Downloader) fetchChengguoCatalogPage(ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
	if page < 1 || page > 100000 || len(query) > 200 || (category != "" && !validChengguoID(category)) {
		return nil, false, errors.New("橙果目录查询参数无效")
	}
	site := d.providerBaseURL(sourceChengguo)
	route := "/api/v1/dramas?page=" + strconv.Itoa(page)
	rowsKey := "items"
	if query != "" {
		route = "/api/v1/search?q=" + url.QueryEscape(query) + "&page=" + strconv.Itoa(page)
		rowsKey = "dramas"
	} else if category != "" {
		route += "&tag=" + url.QueryEscape(category)
	}
	data, err := d.chengguoPost(ctx, route, map[string]any{}, false)
	if err != nil {
		return nil, false, err
	}
	rows, valid := data[rowsKey].([]any)
	if !valid || len(rows) > 500 {
		return nil, false, errors.New("橙果目录数据格式无效")
	}
	items := make([]Drama, 0, len(rows))
	seen := map[string]bool{}
	for _, entry := range rows {
		row, _ := entry.(map[string]any)
		drama, err := chengguoDramaFromRow(row, site)
		if err != nil {
			return nil, false, err
		}
		if seen[drama.ID] {
			continue
		}
		seen[drama.ID] = true
		items = append(items, drama)
	}
	hasMore := len(rows) > 0
	if pagination, ok := data["pagination"].(map[string]any); ok {
		if current, valid := huangjuInteger(pagination["page"], 100000); valid && current > 0 {
			if totalPages, ok := huangjuInteger(pagination["total_pages"], 100000); ok {
				hasMore = current < totalPages
			}
		}
	}
	return items, hasMore, nil
}

func (d *Downloader) fetchChengguoDetail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	if !validChengguoID(sourceID) {
		return Drama{}, nil, errors.New("橙果剧集地址无效")
	}
	site := d.providerBaseURL(sourceChengguo)
	data, err := d.chengguoPost(ctx, "/api/v1/dramas/"+url.PathEscape(sourceID), map[string]any{}, false)
	if err != nil {
		return Drama{}, nil, err
	}
	drama, err := chengguoDramaFromRow(data, site)
	if err != nil {
		return Drama{}, nil, err
	}
	rows, valid := data["episodes"].([]any)
	if !valid || len(rows) > 5000 {
		return Drama{}, nil, errors.New("橙果分集目录格式无效")
	}
	chapters := make([]Chapter, 0, len(rows))
	seen := map[int]bool{}
	for index, entry := range rows {
		episode, _ := entry.(map[string]any)
		number := index + 1
		if value, ok := huangjuInteger(episode["number"], 100000); ok && value > 0 {
			number = value
		}
		if seen[number] {
			continue
		}
		seen[number] = true
		chapters = append(chapters, Chapter{
			ID: providerChapterID(sourceChengguo, sourceID, strconv.Itoa(number)), Source: sourceChengguo,
			Title: "第 " + strconv.Itoa(number) + " 集", CurrentEpisode: json.RawMessage(strconv.Itoa(number)),
			VideoURL: chengguoPlayScheme + sourceID + "/" + strconv.Itoa(number), Referer: site + "/",
		})
	}
	if len(chapters) == 0 {
		return Drama{}, nil, errors.New("橙果分集目录为空")
	}
	sort.SliceStable(chapters, func(i, j int) bool {
		left, _ := strconv.Atoi(chapters[i].EpisodeString(i + 1))
		right, _ := strconv.Atoi(chapters[j].EpisodeString(j + 1))
		return left < right
	})
	if drama.TotalEpisode == nil || valueEmpty(drama.TotalEpisode) {
		drama.TotalEpisode, drama.EpisodeCount = len(chapters), len(chapters)
	}
	return drama, chapters, nil
}

func (d *Downloader) resolveChengguoMedia(ctx context.Context, task Task) (providerMedia, error) {
	source, dramaID, valid := splitProviderDramaID(task.DramaID)
	if !valid || source != sourceChengguo || !validChengguoID(dramaID) || task.Chapter.Source != sourceChengguo {
		return providerMedia{}, errors.New("橙果播放分集信息无效，请刷新详情")
	}
	if !strings.HasPrefix(task.Chapter.VideoURL, chengguoPlayScheme) {
		return providerMedia{}, errors.New("橙果播放分集地址无效")
	}
	rest := strings.TrimPrefix(task.Chapter.VideoURL, chengguoPlayScheme)
	sourceID, numberText, ok := strings.Cut(rest, "/")
	if !ok || sourceID != dramaID || !validChengguoID(sourceID) || strings.Contains(numberText, "/") {
		return providerMedia{}, errors.New("橙果播放分集地址无效")
	}
	number, err := strconv.Atoi(numberText)
	if err != nil || number < 1 || number > 100000 || strconv.Itoa(number) != numberText || task.Chapter.ID != providerChapterID(sourceChengguo, sourceID, numberText) {
		return providerMedia{}, errors.New("橙果播放分集编号无效")
	}
	site := d.providerBaseURL(sourceChengguo)
	route := "/api/v1/dramas/" + url.PathEscape(sourceID) + "/episodes/" + strconv.Itoa(number)
	data, err := d.chengguoPost(ctx, route, map[string]any{}, true)
	if err != nil {
		return providerMedia{}, err
	}
	media, _ := data["media"].(map[string]any)
	address := mapString(media, "source_url")
	if address == "" {
		address = mapString(media, "h265_url")
	}
	if !isProviderHTTPMediaURL(address) {
		return providerMedia{}, errors.New("橙果未返回可播放地址")
	}
	result := providerMedia{URL: address, Referer: site + "/"}
	if episode, ok := data["episode"].(map[string]any); ok {
		if seconds, valid := huangjuInteger(episode["duration_seconds"], 86400); valid && seconds > 0 {
			result.Duration = time.Duration(seconds) * time.Second
		}
	}
	parsed, parseErr := url.Parse(address)
	if parseErr == nil && strings.HasSuffix(strings.ToLower(parsed.Path), ".m3u8") {
		result.Playlist, result.URL, err = d.fetchMediaPlaylist(ctx, result.URL, result.Referer)
		if err != nil {
			return providerMedia{}, fmt.Errorf("获取橙果播放列表失败：%w", err)
		}
		result.Duration = m3u8Duration(result.Playlist)
	}
	return result, nil
}
