package core

import (
	"context"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// hostObject 构造注入到 JS 的 host 对象。
func (p *jsProvider) hostObject(instance *jsInstance) *goja.Object {
	id := p.module.manifest.ID
	vm := instance.vm
	host := vm.NewObject()
	// 模块可返回的封面域名白名单；为空时不限制。
	allowedCoverHosts := allowedModuleCoverHosts()

	bytesOf := func(value goja.Value) []byte {
		if text, ok := value.Export().(string); ok {
			return []byte(text)
		}
		var data []byte
		if err := vm.ExportTo(value, &data); err != nil {
			panic(vm.NewTypeError("需要字符串或字节数组"))
		}
		return append([]byte(nil), data...)
	}
	bytesValue := func(data []byte) goja.Value {
		array, err := vm.New(vm.Get("Uint8Array"), vm.ToValue(vm.NewArrayBuffer(data)))
		if err != nil {
			panic(err)
		}
		return array
	}

	members := map[string]any{
		"source":  id,
		"dramaId": func(sourceID string) string { return providerDramaID(id, sourceID) },
		"chapterId": func(sourceID, key string) string {
			return providerChapterID(id, sourceID, key)
		},
		"fetch": func(request jsFetchRequest) (jsFetchResponse, error) {
			return p.fetch(instance.ctx, request)
		},
		// 封面地址校验：只返回开关，模块自己决定如何降级。
		"coverAllowed": func(url string) bool {
			return moduleCoverHostAllowed(url, allowedCoverHosts)
		},
		"base64Encode": func(data goja.Value) string {
			return base64.StdEncoding.EncodeToString(bytesOf(data))
		},
		"base64Decode": func(text string) (goja.Value, error) {
			data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
			if err != nil {
				return nil, errors.New("Base64 内容无效")
			}
			return bytesValue(data), nil
		},
		"md5": func(data goja.Value) goja.Value {
			sum := md5.Sum(bytesOf(data))
			return bytesValue(sum[:])
		},
		"sha256": func(data goja.Value) goja.Value {
			sum := sha256.Sum256(bytesOf(data))
			return bytesValue(sum[:])
		},
		"randomBytes": func(size int) (goja.Value, error) {
			if size < 0 || size > 1024 {
				return nil, errors.New("随机字节数无效")
			}
			data := make([]byte, size)
			if _, err := rand.Read(data); err != nil {
				return nil, err
			}
			return bytesValue(data), nil
		},
		"aesCbcEncrypt": func(key, iv, data goja.Value) (goja.Value, error) {
			block, vector, plain, err := moduleCBC(bytesOf(key), bytesOf(iv), bytesOf(data))
			if err != nil {
				return nil, err
			}
			cipher.NewCBCEncrypter(block, vector).CryptBlocks(plain, plain)
			return bytesValue(plain), nil
		},
		"aesCbcDecrypt": func(key, iv, data goja.Value) (goja.Value, error) {
			block, vector, sealed, err := moduleCBC(bytesOf(key), bytesOf(iv), bytesOf(data))
			if err != nil {
				return nil, err
			}
			cipher.NewCBCDecrypter(block, vector).CryptBlocks(sealed, sealed)
			return bytesValue(sealed), nil
		},
		"hmacSha256": func(key, data goja.Value) goja.Value {
			mac := hmac.New(sha256.New, bytesOf(key))
			_, _ = mac.Write(bytesOf(data))
			return bytesValue(mac.Sum(nil))
		},
		"gzip": func(data goja.Value) (goja.Value, error) {
			var buffer bytes.Buffer
			writer := gzip.NewWriter(&buffer)
			if _, err := writer.Write(bytesOf(data)); err != nil {
				return nil, err
			}
			if err := writer.Close(); err != nil {
				return nil, err
			}
			return bytesValue(buffer.Bytes()), nil
		},
		"gunzip": func(data goja.Value) (goja.Value, error) {
			raw := bytesOf(data)
			reader, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				return nil, errors.New("gzip 数据无效")
			}
			defer reader.Close()
			plain, err := io.ReadAll(io.LimitReader(reader, moduleHTMLBytes))
			if err != nil {
				return nil, err
			}
			return bytesValue(plain), nil
		},
		"ecdhP256KeyPair": func() (*goja.Object, error) {
			private, err := ecdh.P256().GenerateKey(rand.Reader)
			if err != nil {
				return nil, err
			}
			pair := vm.NewObject()
			_ = pair.Set("privateKey", bytesValue(private.Bytes()))
			_ = pair.Set("publicKey", bytesValue(private.PublicKey().Bytes()))
			return pair, nil
		},
		"ecdhP256": func(privateKey, publicKey goja.Value) (goja.Value, error) {
			private, err := ecdh.P256().NewPrivateKey(bytesOf(privateKey))
			if err != nil {
				return nil, errors.New("ECDH 私钥无效")
			}
			public, err := ecdh.P256().NewPublicKey(bytesOf(publicKey))
			if err != nil {
				return nil, errors.New("ECDH 公钥无效")
			}
			secret, err := private.ECDH(public)
			if err != nil {
				return nil, errors.New("ECDH 协商失败")
			}
			return bytesValue(secret), nil
		},
		"hkdfSha256": func(secret, salt, info goja.Value, length int) (goja.Value, error) {
			if length < 1 || length > 1024 {
				return nil, errors.New("HKDF 长度无效")
			}
			key, err := hkdf.Key(sha256.New, bytesOf(secret), bytesOf(salt), string(bytesOf(info)), length)
			if err != nil {
				return nil, err
			}
			return bytesValue(key), nil
		},
		"aesGcmSeal": func(key, nonce, plain, extra goja.Value) (goja.Value, error) {
			aead, vector, err := moduleGCM(bytesOf(key), bytesOf(nonce))
			if err != nil {
				return nil, err
			}
			return bytesValue(aead.Seal(nil, vector, bytesOf(plain), bytesOf(extra))), nil
		},
		"aesGcmOpen": func(key, nonce, sealed, extra goja.Value) (goja.Value, error) {
			aead, vector, err := moduleGCM(bytesOf(key), bytesOf(nonce))
			if err != nil {
				return nil, err
			}
			plain, err := aead.Open(nil, vector, bytesOf(sealed), bytesOf(extra))
			if err != nil {
				return nil, errors.New("AES-GCM 校验失败")
			}
			return bytesValue(plain), nil
		},
		"sha1": func(data goja.Value) goja.Value {
			sum := sha1.Sum(bytesOf(data))
			return bytesValue(sum[:])
		},
		"resolveDNS": func(name string) ([]string, error) {
			return resolveModuleDNS(instance.ctx, name)
		},
		"utf8": func(data goja.Value) string { return string(bytesOf(data)) },
		"bytes": func(text string) goja.Value { return bytesValue([]byte(text)) },
		// 模块私有存储：跨调用保留，按模块隔离，带 TTL 与配额。
		"storageGet": func(key string) string {
			value, _ := moduleStorageFor(id).Get(key)
			return value
		},
		"storageSet": func(key, value string, ttlSec int) error {
			return moduleStorageFor(id).Set(key, value, time.Duration(ttlSec)*time.Second)
		},
		"storageDelete": func(key string) {
			moduleStorageFor(id).Delete(key)
		},
	}
	for name, value := range members {
		if err := host.Set(name, value); err != nil {
			panic(err)
		}
	}
	return host
}

func moduleCBC(key, iv, data []byte) (cipher.Block, []byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, errors.New("AES 密钥长度无效")
	}
	if len(iv) != aes.BlockSize || len(data)%aes.BlockSize != 0 {
		return nil, nil, nil, errors.New("AES 向量或数据长度无效")
	}
	return block, iv, data, nil
}

// fetch 执行 JS 侧发起的请求，复用现有 HTTP 栈与限速器。
func (p *jsProvider) fetch(ctx context.Context, request jsFetchRequest) (jsFetchResponse, error) {
	// 发布页等共享入口：命中缓存直接返回，避免多个源重复拉同一个 URL。
	if key := strings.TrimSpace(request.CacheKey); key != "" {
		if cached, ok := moduleFetchCacheGet(key); ok {
			return cached, nil
		}
	}
	target := strings.TrimSpace(request.URL)
	if !isProviderHTTPMediaURL(target) {
		return jsFetchResponse{}, errors.New("请求地址必须是 http/https")
	}
	if err := p.allowedAddress(target); err != nil {
		return jsFetchResponse{}, err
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > moduleCallTimeout {
		timeout = providerTimeout
	}
	var body io.Reader
	switch {
	case request.BodyBase64 != "":
		raw, decodeErr := base64.StdEncoding.DecodeString(request.BodyBase64)
		if decodeErr != nil {
			return jsFetchResponse{}, errors.New("bodyBase64 无效")
		}
		body = bytes.NewReader(raw)
	case request.Body != "":
		body = strings.NewReader(request.Body)
	}
	prepared, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return jsFetchResponse{}, err
	}
	if pin := strings.TrimSpace(request.ResolveIP); pin != "" {
		original := prepared.URL.Hostname()
		port := prepared.URL.Port()
		if port == "" {
			port = "80"
			if prepared.URL.Scheme == "https" {
				port = "443"
			}
		}
		prepared = prepared.WithContext(context.WithValue(prepared.Context(), pinnedHostKey{}, original))
		prepared = prepared.WithContext(context.WithValue(prepared.Context(), pinnedAddressKey{}, pin))
		prepared.URL.Host = net.JoinHostPort(pin, port)
		prepared.Host = original
	}
	for name, value := range request.Headers {
		prepared.Header.Set(name, value)
	}
	if prepared.Header.Get("User-Agent") == "" {
		prepared.Header.Set("User-Agent", userAgent)
	}
	response, err := p.host.Do(prepared, timeout)
	if err != nil {
		return jsFetchResponse{}, err
	}
	defer response.Body.Close()
	limit := request.Limit
	if limit <= 0 || limit > providerMaxBodyBytes {
		limit = providerMaxBodyBytes
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return jsFetchResponse{}, err
	}
	if int64(len(data)) > limit {
		return jsFetchResponse{}, errors.New("响应超出大小上限")
	}
	result := jsFetchResponse{
		Status: response.StatusCode, Headers: response.Header,
		Body: string(data), BodyBase64: base64.StdEncoding.EncodeToString(data),
	}
	if key := strings.TrimSpace(request.CacheKey); key != "" && response.StatusCode >= 200 && response.StatusCode < 300 {
		moduleFetchCachePut(key, result, request.CacheTTLSec)
	}
	return result, nil
}

func moduleGCM(key, nonce []byte) (cipher.AEAD, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, errors.New("AES 密钥长度无效")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, nil, errors.New("AES-GCM 随机数长度无效")
	}
	return aead, nonce, nil
}
