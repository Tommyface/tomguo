/* global host, exports */
'use strict';
// 51短剧（51duanju）：SSR 站源模块，迁移自 native/core/provider_51duanju.go。
// 站点：https://blue.xdrgcewe.cc
// 播放：GET /api/videos/<id>/play?ep=<n>&page_texts=0 → data.video_url（带 auth_key 的 m3u8）。
// 页面里的 epPlaySrcs 已全为空串，播放地址必须走该接口。
var SITE = 'https://blue.xdrgcewe.cc';
var UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';
var ROUTES = { home: '/', rank: '/ranks/hot/', theater: '/ai-duanju/', 'new': '/newest', manju: '/ai-manju/', huanlian: '/ai-huanlian/', mogai: '/ai-mogai/' };
var CATS = [ { id: 'home', name: '首页推荐' }, { id: 'rank', name: '热播榜' }, { id: 'theater', name: 'AI 短剧' },
  { id: 'new', name: '最新更新' }, { id: 'manju', name: 'AI 漫剧' }, { id: 'huanlian', name: 'AI 换脸' }, { id: 'mogai', name: 'AI 魔改' } ];

function fetchText(url, referer) {
  var r = host.fetch({ url: url, method: 'GET', headers: { 'User-Agent': UA, Referer: referer || SITE + '/' }, timeoutMs: 20000 });
  if (!r || r.status < 200 || r.status >= 400) throw new Error('请求失败：' + (r ? r.status : 'no response'));
  return r.body ? String(r.body) : (r.bodyBase64 ? host.base64Decode(r.bodyBase64) : '');
}
function fetchJSON(url, referer) {
  var t = fetchText(url, referer);
  try { return JSON.parse(t); } catch (e) { throw new Error('接口返回数据格式无效'); }
}
function abs(raw) {
  raw = String(raw || '').trim();
  if (!raw) return '';
  if (/^https?:\/\//i.test(raw)) return raw;
  if (/^\/\//.test(raw)) return 'https:' + raw;
  return SITE + (raw.charAt(0) === '/' ? '' : '/') + raw;
}
function decode(s) {
  return String(s || '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&nbsp;/g, ' ');
}
function strip(s) { return decode(String(s || '').replace(/<[^>]*>/g, '')).replace(/\s+/g, ' ').trim(); }

// 卡片：hg-drama-card / hg-rank-item，数据都在 data-track-* 属性里。
function parseCards(html) {
  var items = [], seen = {};
  var re = /<(?:article|div|li)[^>]*class="[^"]*hg-(?:drama-card|rank-item)[^"]*"[^>]*>/gi, m;
  while ((m = re.exec(html)) !== null) {
    var tagStart = m.index, tag = m[0];
    var idm = /data-track-id="(\d+)"/i.exec(tag);
    if (!idm) continue;
    var id = idm[1];
    if (seen[id]) continue;
    var t = /data-track-title="([^"]*)"/i.exec(tag);
    var title = t ? decode(t[1]).trim() : '';
    // 块内取封面与集数：截到下一个卡片开始
    var rest = html.slice(tagStart, tagStart + 2000);
    if (!title) { var tm = /hg-(?:drama|rank)-item__title[^>]*>\s*([^<]{1,80})/i.exec(rest); if (tm) title = strip(tm[1]); }
    if (!title) continue;
    var cm = /data-src="([^"]+)"/i.exec(rest) || /<img[^>]*src="([^"]+\.(?:jpe?g|png|webp)[^"]*)"/i.exec(rest);
    var cover = cm ? abs(cm[1]) : '';
    var em = /(\d{1,4})\s*集/.exec(rest);
    var eps = em ? parseInt(em[1], 10) : 0;
    var type = /data-track-type-name="([^"]*)"/i.exec(tag);
    seen[id] = true;
    items.push({ id: '51duanju:' + id, source: '51duanju', sourceId: id, title: title, cover: cover,
      episodes: eps, totalEpisode: eps, episodeCount: eps, category: type ? decode(type[1]) : '短剧' });
  }
  return items;
}
function initialData(html) {
  var m = /<script id="videoInitialData"[^>]*>([\s\S]*?)<\/script>/i.exec(html);
  if (!m) return null;
  var raw = decode(m[1]).trim();
  try { return JSON.parse(raw); } catch (e) { return null; }
}

exports.categories = async function () { return CATS.slice(); };

exports.catalogPage = async function (page, category) {
  var p = Number(page) || 1;
  var route = ROUTES[String(category || 'home')] || ROUTES.home;
  var path;
  if (route === '/' || route === '/ranks/hot/') path = p <= 1 ? route : '';
  else path = p <= 1 ? route + (route.slice(-1) === '/' ? '' : '/') : route.replace(/\/$/, '') + '/' + p + '/';
  if (!path) return { items: [], hasMore: false };
  var html = fetchText(SITE + path, SITE + '/');
  var items = parseCards(html);
  return { items: items, hasMore: items.length > 0 && /rel="next"|>\s*下一页\s*</.test(html) };
};

exports.search = async function (page, query) {
  var q = String(query || '').trim();
  if (!q) return { items: [], hasMore: false };
  var p = Number(page) || 1;
  // 站点 JSON 搜索接口（参数名是 q）；HTML 搜索页 /search/video/<kw>/ 在长时间请求下会超时。
  var res = fetchJSON(SITE + '/api/search?q=' + encodeURIComponent(q) + '&page=' + p, SITE + '/');
  var dd = (res && res.data) || {};
  var rows = dd.items || [];
  var items = [];
  rows.forEach(function (row) {
    var id = String(row.id || '').replace(/[^\d]/g, '');
    if (!id) return;
    var eps = Number(row.total_episodes || row.episode_count || row.totalEpisodes) || 0;
    var tags = Array.isArray(row.tags) ? row.tags.slice(0, 6).map(function (t) { return String(t.name || t); }) : [];
    items.push({ id: '51duanju:' + id, source: '51duanju', sourceId: id, title: String(row.title || '').trim(),
      description: String(row.description || ''), cover: abs(row.cover || row.cover_url),
      episodes: eps, totalEpisode: eps, episodeCount: eps, category: '短剧', tags: tags });
  });
  var pager = dd.pagination || {};
  return { items: items, hasMore: Number(pager.total_page || pager.totalPage || 0) > p };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').replace(/^51duanju:/, '').replace(/[^\d]/g, '');
  if (!id) throw new Error('51短剧剧集 ID 无效');
  var html = fetchText(SITE + '/video/' + id + '/', SITE + '/');
  var data = initialData(html);
  // 分集号：优先 data-ep-id 按钮，其次集长表
  var nums = {}, mm, re = /data-ep-id="(\d+)"/gi;
  while ((mm = re.exec(html)) !== null) nums[parseInt(mm[1], 10)] = true;
  if (!Object.keys(nums).length && data && data.episodeDurationLabels) {
    Object.keys(data.episodeDurationLabels).forEach(function (k) { var n = parseInt(k, 10); if (n > 0) nums[n] = true; });
  }
  var idx = Object.keys(nums).map(Number).filter(function (n) { return n > 0; }).sort(function (a, b) { return a - b; });
  if (!idx.length) throw new Error('51短剧该剧暂无可用分集');
  var chapters = idx.map(function (n) {
    return { id: '51duanju:' + id + ':' + n, source: '51duanju', sourceId: String(n),
      title: '第' + n + '集', currentEpisode: String(n), videoUrl: '51duanju-play://' + id + '/' + n,
      pageUrl: SITE + '/video/' + id + '/ep-' + n + '/', referer: SITE + '/' };
  });
  var title = (data && data.title) || ('短剧' + id);
  var cover = data ? abs(data.coverSrc || data.posterSrc) : '';
  var desc = data ? String(data.description || '') : '';
  var tags = data && Array.isArray(data.tags) ? data.tags.slice(0, 8) : [];
  var cat = data && Array.isArray(data.breadcrumb) && data.breadcrumb[0] ? String(data.breadcrumb[0].name || '短剧') : '短剧';
  return { id: '51duanju:' + id, source: '51duanju', sourceId: id, title: title, description: desc,
    cover: cover, episodes: chapters.length, totalEpisode: chapters.length, episodeCount: chapters.length,
    category: cat, tags: tags, chapters: chapters };
};

exports.resolveMedia = async function (task) {
  var ch = (task && task.chapter) || {};
  var parts = String(ch.id || '').split(':');
  var id = parts.length >= 3 ? parts[1] : '';
  var ep = parseInt(parts.length >= 3 ? parts[2] : '', 10);
  if (!id) {
    var vm = /^51duanju-play:\/\/(\d+)\/(\d+)$/.exec(String(ch.videoUrl || ''));
    if (vm) { id = vm[1]; ep = parseInt(vm[2], 10); }
  }
  if (!ep || ep < 1) ep = parseInt(String(ch.currentEpisode || '1'), 10) || 1;
  if (!id) { var dm = /^51duanju:(\d+)/.exec(String((task && task.dramaId) || ch.id || '')); if (dm) id = dm[1]; }
  if (!id) throw new Error('51短剧播放分集信息无效，请刷新详情');
  var res = fetchJSON(SITE + '/api/videos/' + encodeURIComponent(id) + '/play?ep=' + ep + '&page_texts=0', SITE + '/');
  var url = res && res.data && res.data.video_url ? String(res.data.video_url) : '';
  if (!/^https?:\/\//i.test(url)) throw new Error('51短剧第' + ep + '集暂未返回可用播放地址');
  return { url: url, referer: SITE + '/' };
};
