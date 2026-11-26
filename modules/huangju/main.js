/* global host, exports */
'use strict';
// 剧果（huangju）：JSON API 站源模块，迁移自 native/core/provider_huangju*.go。
// API：https://api.huangju.net，访客授权 POST /auth/guest {deviceId} → token。
var API = 'https://api.huangju.net';
var SITE = 'https://video.huangju.net';
var UA = 'Mozilla/5.0 (Linux; Android 11; Pixel 5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.91 Mobile Safari/537.36';
var TOKEN = '';
var DEVICE = '';
function deviceId() {
  if (DEVICE) return DEVICE;
  if (host.uuid) { try { DEVICE = String(host.uuid() || ''); } catch (e) {} }
  if (!DEVICE) { DEVICE = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c){ var r = Math.random()*16|0; return (c==='x'?r:(r&3|8)).toString(16); }); }
  return DEVICE;
}
function req(method, path, body) {
  var headers = { 'User-Agent': UA, 'Accept': 'application/json, text/plain, */*', 'Accept-Language': 'zh-CN,zh;q=0.9', 'Referer': SITE + '/', 'Origin': SITE };
  if (body) headers['Content-Type'] = 'application/json';
  if (TOKEN) headers['Authorization'] = 'Bearer ' + TOKEN;
  var r = host.fetch({ url: API + path, method: method, headers: headers, body: body ? JSON.stringify(body) : undefined, timeoutMs: 15000 });
  if (!r || r.status < 200 || r.status >= 400) throw new Error('剧果请求失败：' + (r ? r.status : 'no response'));
  var text = r.body ? String(r.body) : (r.bodyBase64 ? host.base64Decode(r.bodyBase64) : '');
  try { return JSON.parse(text); } catch (e) { throw new Error('剧果返回数据格式无效'); }
}
function auth() {
  if (TOKEN) return;
  var row = req('POST', '/auth/guest', { deviceId: deviceId() });
  TOKEN = row.token || (row.data && row.data.token) || '';
  if (!TOKEN) throw new Error('剧果未返回有效访客授权');
}
function hostOf(u) { var m = /^(https?:\/\/[^\/]+)/i.exec(String(u||'')); return m ? m[1] : SITE; }
function cover(raw) {
  raw = String(raw || '');
  if (/^\/\//.test(raw)) return 'https:' + raw;
  if (/^https?:\/\//i.test(raw)) return raw;
  if (!raw) return '';
  return hostOf(SITE) + (raw.charAt(0)==='/' ? '' : '/') + raw;
}
function dramaItem(row) {
  var id = String(row.id || ''), slug = String(row.slug || '');
  if (!id || /[\\\/:?#%"<>|]/.test(id)) return null;
  var cats = [];
  var add = function(v){ var n = typeof v === 'string' ? v : (v && v.name) || ''; if (n && cats.indexOf(n)<0) cats.push(n); };
  add(row.category);
  if (Array.isArray(row.categories)) row.categories.forEach(add);
  var title = String(row.title || '').trim();
  if (!title) return null;
  var eps = Number(row.totalEpisodes) || 0;
  var tags = cats.slice();
  if (row.year) tags.push(row.year + '年');
  if (row.region) tags.push(String(row.region));
  if (Number(row.score) > 0) tags.push('评分 ' + row.score);
  var status = row.status === 'completed' ? 'finished' : (row.status === 'ongoing' ? 'ongoing' : '');
  return { id: 'huangju:' + (slug ? slug + '-' + id : id), source: 'huangju', sourceId: slug ? slug + '-' + id : id,
    title: title, description: String(row.description || ''), cover: cover(row.coverUrl),
    episodes: eps, totalEpisode: eps, episodeCount: eps, category: cats[0] || '短剧', tags: tags,
    releaseStatus: status, score: Number(row.score) > 0 ? String(row.score) : '' };
}
exports.categories = async function () { return [{ id: '', name: '全部' }, { id: '@new', name: '最新' }]; };
exports.catalogPage = async function (page, category) {
  auth();
  var p = Number(page) || 1;
  var qs = '?page=' + p;
  if (category === '@new') qs += '&sort=new';
  else if (!category) qs += '&sort=hot';
  else qs += '&category=' + encodeURIComponent(category);
  var row = req('GET', '/dramas' + qs);
  var rows = row.items || [];
  var items = [];
  for (var i=0;i<rows.length;i++) { var d = dramaItem(rows[i]); if (d) items.push(d); }
  var pageSize = Number(row.pageSize) || 20;
  var hasMore = row.total ? p < Math.ceil(Number(row.total)/pageSize) : rows.length >= pageSize;
  return { items: items, hasMore: hasMore };
};
exports.search = async function (page, query) {
  auth();
  var q = String(query || '').trim();
  if (!q) return { items: [], hasMore: false };
  var row = req('GET', '/dramas?page=' + (Number(page)||1) + '&q=' + encodeURIComponent(q));
  var items = [];
  for (var i=0;i<(row.items||[]).length;i++) { var d = dramaItem(row.items[i]); if (d) items.push(d); }
  return { items: items, hasMore: false };
};
exports.detail = async function (sourceId) {
  auth();
  var sid = String(sourceId || '').replace(/^huangju:/, '');
  if (!sid || /[\\\/:?#%"<>|]/.test(sid)) throw new Error('剧果剧集地址无效');
  var row = req('GET', '/dramas/' + encodeURIComponent(sid));
  var d = dramaItem(row);
  if (!d) throw new Error('剧果返回的剧集信息不完整');
  d.id = 'huangju:' + sid; d.sourceId = sid;
  var eps = row.episodes || [];
  var chapters = [];
  for (var i=0;i<eps.length;i++) {
    var e = eps[i];
    if (e.playable === false) continue;
    var eid = String(e.id || '');
    if (!eid || /[\\\/:?#%"<>|]/.test(eid)) continue;
    var num = Number(e.epNo) || i + 1;
    chapters.push({ id: 'huangju:' + sid + ':' + eid, source: 'huangju', sourceId: eid,
      title: '第 ' + num + ' 集', videoUrl: 'huangju-play://' + encodeURIComponent(eid),
      currentEpisode: String(num), pageUrl: API + '/play/' + encodeURIComponent(eid), referer: SITE + '/' });
  }
  chapters.sort(function(a,b){ return Number(a.currentEpisode) - Number(b.currentEpisode); });
  d.episodes = chapters.length; d.totalEpisode = chapters.length; d.episodeCount = chapters.length; d.chapters = chapters;
  return d;
};
exports.resolveMedia = async function (task) {
  auth();
  var ch = task && task.chapter || {};
  var eid = String(ch.sourceId || String(ch.videoUrl || '').replace('huangju-play://','') || '');
  if (!eid || /[\\\/:?#%"<>|]/.test(eid)) throw new Error('剧果分集地址无效');
  var row = req('GET', '/play/' + encodeURIComponent(eid));
  var url = String(row.url || '');
  if (!/^https?:\/\//i.test(url)) throw new Error('剧果未返回有效播放地址');
  return { url: url, referer: SITE + '/' };
};
