/* global host, exports */
'use strict';
// 河马剧场（hema）：Next.js 站源模块，迁移自 native/core/provider_hema.go。
// 站点：https://www.kuaikaw.cn
// 关键：网页详情页 <script id="__NEXT_DATA__"> 内嵌 chapterList[].chapterVideoVo.mp4，
// 播放地址无需走 App 加密门户接口（那条路已返回错误码）。
var SITE = 'https://www.kuaikaw.cn';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';

function fetchText(url, referer) {
  var r = host.fetch({ url: url, method: 'GET', headers: { 'User-Agent': UA, Referer: referer || SITE + '/' }, timeoutMs: 20000 });
  if (!r || r.status < 200 || r.status >= 400) throw new Error('请求失败：' + (r ? r.status : 'no response'));
  return r.body ? String(r.body) : (r.bodyBase64 ? host.base64Decode(r.bodyBase64) : '');
}
function nextData(html) {
  var m = /<script id="__NEXT_DATA__" type="application\/json">([\s\S]*?)<\/script>/i.exec(html);
  if (!m) return null;
  try { return JSON.parse(m[1]); } catch (e) { return null; }
}
function props(html) {
  var d = nextData(html);
  return d && d.props && d.props.pageProps ? d.props.pageProps : null;
}
function abs(raw) {
  raw = String(raw || '').trim();
  if (!raw) return '';
  if (/^https?:\/\//i.test(raw)) return raw;
  if (/^\/\//.test(raw)) return 'https:' + raw;
  return SITE + (raw.charAt(0) === '/' ? '' : '/') + raw;
}
function num(v) { var n = parseInt(String(v == null ? '' : v), 10); return isNaN(n) ? 0 : n; }
function bookId(id) { var s = String(id || '').trim(); return /^\d{6,20}$/.test(s) ? s : ''; }

function toDrama(row) {
  if (!row) return null;
  var id = bookId(row.bookId);
  var title = String(row.bookName || '').trim();
  if (!id || !title) return null;
  var tags = [];
  if (Array.isArray(row.bookTypeThree)) row.bookTypeThree.forEach(function (t) { if (t && t.name && tags.length < 8) tags.push(String(t.name)); });
  if (row.actor) tags.push('主演 ' + String(row.actor).slice(0, 16));
  if (row.actress) tags.push('主演 ' + String(row.actress).slice(0, 16));
  var eps = num(row.totalChapterNum) || num(row.chapterNum);
  return { id: 'hema:' + id, source: 'hema', sourceId: id, title: title,
    description: String(row.introduction || row.desc || ''), cover: abs(row.coverWap || row.cover || row.coverUrl),
    episodes: eps, totalEpisode: eps, episodeCount: eps,
    category: tags[0] || '短剧', tags: tags };
}

exports.categories = async function () {
  var p = props(fetchText(SITE + '/browse', SITE + '/'));
  var rows = (p && p.typeThreeList) || [];
  var out = [{ id: '', name: '全部' }];
  rows.forEach(function (r) {
    var id = String((r && (r.typeThreeId || r.id)) || '').trim();
    var name = String((r && r.name) || '').trim();
    if (id && name && !/^\d{5,}$/.test(name)) out.push({ id: id, name: name });
  });
  return out;
};

exports.catalogPage = async function (page, category) {
  var p = Number(page) || 1;
  var cat = String(category || '').trim() || '0';
  if (!/^\d{1,10}$/.test(cat)) cat = '0';
  var data = props(fetchText(SITE + '/browse/' + cat + '/' + p, SITE + '/browse'));
  var rows = (data && data.bookList) || [];
  var items = [];
  var seen = {};
  rows.forEach(function (row) {
    var d = toDrama(row);
    if (d && !seen[d.sourceId]) { seen[d.sourceId] = true; items.push(d); }
  });
  var total = num(data && data.pages);
  return { items: items, hasMore: total > 0 ? p < total : rows.length >= 20 };
};

exports.search = async function (page, query) {
  var q = String(query || '').trim();
  if (!q) return { items: [], hasMore: false };
  var p = Number(page) || 1;
  var r = host.fetch({ url: SITE + '/seo/video/6007', method: 'POST',
    headers: { 'User-Agent': UA, 'Content-Type': 'application/json', Referer: SITE + '/search' },
    body: JSON.stringify({ sourceType: 1, keyword: q, index: p }), timeoutMs: 20000 });
  if (!r || r.status < 200 || r.status >= 400) return { items: [], hasMore: false };
  var text = r.body ? String(r.body) : (r.bodyBase64 ? host.base64Decode(r.bodyBase64) : '');
  var j; try { j = JSON.parse(text); } catch (e) { return { items: [], hasMore: false }; }
  var rows = (j && j.data && (j.data.bookList || j.data.list)) || [];
  var items = [];
  rows.forEach(function (row) { var d = toDrama(row); if (d) items.push(d); });
  return { items: items, hasMore: false };
};

exports.detail = async function (sourceId) {
  var id = bookId(String(sourceId || '').replace(/^hema:/, ''));
  if (!id) throw new Error('河马剧场剧集 ID 无效');
  var p = props(fetchText(SITE + '/drama/' + id, SITE + '/'));
  if (!p) throw new Error('河马剧场详情页结构异常');
  var info = p.bookInfoVo || {};
  var list = Array.isArray(p.chapterList) ? p.chapterList : [];
  if (!list.length) throw new Error('河马剧场该剧暂无可用分集');
  var chapters = [];
  list.forEach(function (c, i) {
    var cid = String((c && c.chapterId) || '').trim();
    if (!/^\d{6,20}$/.test(cid)) return;
    var idx = num(c.chapterIndex) || (i + 1);
    var vo = c.chapterVideoVo || {};
    var media = [vo.mp4, vo.mp4720p, vo.m3u8].filter(function (u) { return /^https?:\/\//i.test(String(u || '')); })[0] || '';
    chapters.push({ id: 'hema:' + id + ':' + cid, source: 'hema', sourceId: cid,
      title: String(c.chapterName || ('第 ' + idx + ' 集')), currentEpisode: String(idx),
      videoUrl: 'hema-play://' + cid, mediaUrl: media, referer: SITE + '/' });
  });
  chapters.sort(function (a, b) { return Number(a.currentEpisode) - Number(b.currentEpisode); });
  var d = toDrama(info) || { id: 'hema:' + id, source: 'hema', sourceId: id, title: '河马剧场' + id };
  d.id = 'hema:' + id; d.sourceId = id;
  d.episodes = chapters.length; d.totalEpisode = chapters.length; d.episodeCount = chapters.length;
  d.chapters = chapters;
  return d;
};

exports.resolveMedia = async function (task) {
  var ch = (task && task.chapter) || {};
  // 详情阶段已把 chapterVideoVo.mp4 带下来，优先直用。
  if (/^https?:\/\//i.test(String(ch.mediaUrl || ''))) return { url: String(ch.mediaUrl), referer: SITE + '/' };
  // resolve task 只带 dramaId 与 chapter（chapter.id = hema:<bookId>:<chapterId>）。
  var chId = String(ch.id || '');
  var parts = chId.split(':');
  var id = bookId(parts.length >= 3 ? parts[1] : (String((task && task.dramaId) || '').split(':').pop()));
  var cid = String(ch.sourceId || (parts.length >= 3 ? parts[2] : '') || String(ch.videoUrl || '').replace('hema-play://', '') || '');
  if (!id || !/^\d{6,20}$/.test(cid)) throw new Error('河马剧场播放分集信息无效');
  var p = props(fetchText(SITE + '/drama/' + id, SITE + '/'));
  var list = (p && Array.isArray(p.chapterList)) ? p.chapterList : [];
  for (var i = 0; i < list.length; i++) {
    if (String(list[i].chapterId) !== cid) continue;
    var vo = list[i].chapterVideoVo || {};
    var u = [vo.mp4, vo.mp4720p, vo.m3u8].filter(function (x) { return /^https?:\/\//i.test(String(x || '')); })[0];
    if (u) return { url: u, referer: SITE + '/' };
    break;
  }
  throw new Error('河马剧场该分集暂无可播放地址');
};
