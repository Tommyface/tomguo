/* global host, exports */
'use strict';
// 发果（faguo）：MacCMS 站源模块，迁移自 native/core/provider_maccms.go。
// 站点：https://www.xzyx168.com
var SITE = 'https://www.xzyx168.com';
var UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';
var DETAIL_RE = new RegExp('/xzyxvd/(\d+)\.html', 'i');
var PLAY_RE = new RegExp('/xzyxvp/(\d+)n(\d+)m(\d+)\.html', 'i');

// 用 host.parse 拿到 DOM；没有 DOM 适配器时退回正则。
function doc(html) {
  if (host.parse) { try { return host.parse(html); } catch (e) {} }
  return null;
}
function textOf(node) {
  if (!node) return '';
  if (typeof node.text === 'function') return String(node.text() || '').trim();
  if (typeof node.textContent === 'string') return node.textContent.trim();
  return '';
}
function attr(node, name) {
  if (!node) return '';
  if (typeof node.attr === 'function') return String(node.attr(name) || '');
  if (node.attribs && node.attribs[name]) return String(node.attribs[name]);
  return '';
}
function findAll(node, sel) {
  if (!node) return [];
  if (typeof node.find === 'function') { try { return node.find(sel).toArray ? node.find(sel).toArray() : node.find(sel); } catch (e) {} }
  if (typeof host.select === 'function') { try { return host.select(htmlHolder, sel) || []; } catch (e) {} }
  return [];
}
var htmlHolder = '';
function fetchText(url, referer) {
  var r = host.fetch({ url: url, method: 'GET', headers: { 'User-Agent': UA, Referer: referer || SITE + '/' }, timeoutMs: 15000 });
  if (!r || r.status < 200 || r.status >= 400) throw new Error('请求失败：' + (r ? r.status : 'no response'));
  htmlHolder = r.body ? String(r.body) : (r.bodyBase64 ? host.base64Decode(r.bodyBase64) : '');
  return htmlHolder;
}
function abs(raw, base) {
  if (!raw) return '';
  if (/^https?:/i.test(raw)) return raw;
  if (/^\/\//.test(raw)) return 'https:' + raw;
  if (!base) base = SITE + '/';
  var u = host.parseUrl ? host.parseUrl(base) : null;
  var origin = u ? (u.protocol + '//' + u.host) : SITE;
  return raw.charAt(0) === '/' ? origin + raw : origin + '/' + raw;
}
function detailId(href) {
  var m = DETAIL_RE.exec(href || '');
  return m ? m[1] : '';
}
function decodeEntities(s) {
  return String(s || '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&nbsp;/g, ' ').replace(/&#x([0-9a-f]+);/gi, function(_, h) { return String.fromCharCode(parseInt(h, 16)); }).replace(/&#(\d+);/g, function(_, d) { return String.fromCharCode(parseInt(d, 10)); });
}
function strip(s) { return decodeEntities(String(s || '').replace(/<[^>]*>/g, '')).replace(/\s+/g, ' ').trim(); }
function epNumber(s) { var m = /(?:第|episode\s*)?(\d{1,4})(?:集|话|期)?/i.exec(String(s || '')); return m ? parseInt(m[1], 10) : 0; }

// 正则兜底解析：从 <a href=详情> 往上取整块卡片，抽标题与封面。
function parseCardsRegex(html) {
  var items = [], seen = {};
  var re = /<a\b[^>]*href=["']([^"']+)["'][^>]*>([\s\S]{0,1200}?)<\/a>/gi, m;
  while ((m = re.exec(html)) !== null) {
    var id = detailId(m[1]);
    if (!id || seen[id]) continue;
    var inner = m[2];
    var title = strip((inner.match(/title=["']([^"']+)["']/i) || [])[1] || inner.replace(/<[^>]+>/g, ' '));
    if (!title || title.length > 120) continue;
    seen[id] = true;
    var cm = /(?:src|data-original|data-src)=["']([^"']+\.(?:jpg|jpeg|png|webp)[^"']*)["']/i.exec(inner) || /(?:src|data-original|data-src)=["']([^"']+)["']/i.exec(inner);
    var cover = cm ? abs(cm[1], SITE + '/') : '';
    var rm = /全?(\d{1,3})集/.exec(inner);
    var count = rm ? parseInt(rm[1], 10) : 0;
    if (count > 999) count = 0;
    items.push({ id: 'faguo:' + id, source: 'faguo', sourceId: id, title: title, cover: cover,
      episodes: count, totalEpisode: count, episodeCount: count });
  }
  return items;
}
function parseCards(html) {
  var d = doc(html);
  if (d && typeof d.find === 'function') {
    var out = [], seen = {};
    var anchors = findAll(d, 'a[href]');
    for (var i = 0; i < anchors.length; i++) {
      var a = anchors[i]; var href = attr(a, 'href'); var id = detailId(href);
      if (!id || seen[id]) continue;
      var title = strip(attr(a, 'title') || textOf(a));
      if (!title || title.length > 120) continue;
      seen[id] = true;
      var cover = ''; var imgs = findAll(a, 'img');
      if (imgs.length) cover = abs(attr(imgs[0], 'data-original') || attr(imgs[0], 'data-src') || attr(imgs[0], 'src'), SITE + '/');
      out.push({ id: 'faguo:' + id, source: 'faguo', sourceId: id, title: title, cover: cover });
    }
    if (out.length) return out;
  }
  return parseCardsRegex(html);
}
function parseDetail(html, pageUrl) {
  var title = '';
  var tm = /<h1[^>]*>([\s\S]{0,300}?)<\/h1>/i.exec(html) || /<title>([\s\S]{0,300}?)<\/title>/i.exec(html);
  if (tm) title = strip(tm[1]).split(/[-_|]/)[0].trim();
  var episodes = [], seen = {};
  var re = /<a\b[^>]*href=["']([^"']+)["'][^>]*>([\s\S]{0,200}?)<\/a>/gi, m;
  while ((m = re.exec(html)) !== null) {
    if (!PLAY_RE.test(m[1])) continue;
    var url = abs(m[1], pageUrl);
    if (seen[url]) continue;
    var t = strip(attrText(m[2]));
    if (!t || /APP|下载|客户端/i.test(t)) continue;
    seen[url] = true;
    var num = epNumber(t) || episodes.length + 1;
    episodes.push({ title: t, url: url, number: num });
  }
  if (!episodes.length) return null;
  episodes.sort(function(a, b) { return a.number - b.number; });
  var ch = episodes.map(function(e) { return { id: 'faguo:' + (host.dramaId ? '' : '') + e.number, source: 'faguo', sourceId: String(e.number), title: e.title,
    videoUrl: 'maccms-play://' + e.number, pageUrl: e.url, referer: SITE + '/', currentEpisode: String(e.number) }; });
  return { id: 'faguo:' + (arguments[2] || ''), source: 'faguo', title: title, cover: '',
    episodes: ch.length, totalEpisode: ch.length, episodeCount: ch.length, chapters: ch };
}
function attrText(s) { var m = /title=["']([^"']+)["']/i.exec(s); return m ? m[1] : s; }
function parsePlayMedia(html, pageUrl) {
  var m = /(?:player_aaaa\s*=\s*|\"url\"\s*:\s*|\"playurl\"\s*:\s*)(?:JSON\.parse\()?["']([^"']+\.(?:m3u8|mp4)[^"']*)["']/i.exec(html)
       || /(["']?(?:https?:)?\/\/[^\s"'<>\\]+?\.(?:m3u8|mp4)(?:\?[^\s"'<>\\]*)?)/i.exec(html);
  if (m) return abs(m[1].replace(/^["']|["']$/g, ''), pageUrl);
  var f = /<iframe\b[^>]*src=["']([^"']+)["']/i.exec(html);
  return f ? abs(f[1], pageUrl) : '';
}

exports.categories = async function () { return [{ id: '', name: '全部' }]; };
exports.catalogPage = async function (page, category) {
  var p = Number(page) || 1;
  var path = (p <= 1 ? '/xzyxvt/duanjun.html' : '');
  if (!path) return { items: [], hasMore: false };
  var html = fetchText(SITE + path, SITE + '/');
  var items = parseCards(html);
  return { items: items, hasMore: items.length > 0 && false };
};
exports.search = async function (page, query) {
  var q = encodeURIComponent(String(query || '').trim());
  if (!q) return { items: [], hasMore: false };
  var path = (function(q){return '/xzyxvc/__Q__-wdyswzqun1num.html';})(q);
  if (!path) return { items: [], hasMore: false };
  var html = fetchText(SITE + path, SITE + '/');
  var items = parseCards(html);
  return { items: items, hasMore: false };
};
exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim().replace(/^faguo:/, '');
  if (!id || !/^\d{1,12}$/.test(id)) throw new Error('剧集 ID 无效');
  var candidates = [(function (id) { return '/xzyxvd/' + id + '.html'; }),
    '/index.php/vod/detail/id/' + id + '.html', '/detail/' + id + '.html', '/vod/' + id + '.html'];
  var last = null;
  for (var i = 0; i < candidates.length; i++) {
    try {
      var url = SITE + candidates[i](id);
      var html = fetchText(url, SITE + '/');
      var d = parseDetail(html, url);
      if (d) { d.id = 'faguo:' + id; return d; }
    } catch (e) { last = e; }
  }
  throw last || new Error('未取到分集');
};
exports.resolveMedia = async function (task) {
  var id = String(task && task.drama && task.drama.id || '').split(':').pop();
  var ch = task && task.chapter || {};
  var url = ch.pageUrl || '';
  if (!url) throw new Error('缺少播放页地址，请刷新详情');
  var seen = {};
  for (var depth = 0; depth < 3; depth++) {
    if (seen[url]) break;
    seen[url] = true;
    var html = fetchText(url, url);
    var media = parsePlayMedia(html, url);
    if (media) return { url: media, referer: url };
    var f = /<iframe\b[^>]*src=["']([^"']+)["']/i.exec(html);
    if (!f) break;
    url = abs(f[1], url);
  }
  throw new Error('未解析到可播放地址，可能线路失效');
};
