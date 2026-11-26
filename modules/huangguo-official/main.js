'use strict';
// 黄果官网（falpius.com）：Next.js 站，数据在 self.__next_f.push 里（多反斜杠转义）。
// 首页 <article><a href="/videos/<slug>">，标题在卡片文本，封面从页面里的绝对图床地址补。
// 详情 /videos/<slug>，episodes[].url 是相对 m3u8，按前端规则走同域 /api/app/vid/h5/m3u8/ 代理。

var SITE = 'https://falpius.com';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';

function fetchText(url) {
  var response = host.fetch({ url: url, method: 'GET', headers: { 'User-Agent': UA, Referer: SITE + '/' }, timeoutMs: 20000 });
  if (!response || response.status < 200 || response.status >= 400) throw new Error('请求失败：' + (response ? response.status : 'no response'));
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}
function unescapeRsc(s) {
  var prev;
  do { prev = s; s = s.replace(/\\(?:\\|")/g, function (m) { return m === '\\\\' ? '\\' : '"'; }).replace(/\\\//g, '/'); } while (s !== prev);
  return s;
}
function parseCards(html) {
  var items = [], seen = {};
  var pattern = /<article\b[^>]*class="[^"]*group[^"]*"[^>]*>\s*<a\b[^>]*href="\/videos\/([a-zA-Z0-9_-]+)"([\s\S]*?)<\/article>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var id = match[1];
    if (seen[id]) continue;
    seen[id] = true;
    var text = String(match[2]).replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim();
    // 卡片文本开头常带箭头/装饰符，先去掉；再从 RSC 里按 seoSlug 取标题更可靠。
    var title = text.replace(/^[>\s]+/, '').replace(/^全\s*\d+\s*集\s*/, '').split(' ').slice(0, 3).join(' ').trim();
    // RSC 里是 \"seoSlug\":\"<id>\"，简单拼接字符串匹配即可，免正则转义地狱。
    var key = '\\"seoSlug\\":\\"' + id + '\\"';
    var pos = html.indexOf(key);
    if (pos >= 0) {
      var after = html.slice(pos + key.length, pos + key.length + 3000);
      var tm = after.match(/title\?"?:\?"([^"\\]+)/);
      if (tm) title = tm[1];
    }
    if (!title) continue;
    var coverMatch = html.match(new RegExp('https?://[^"\\\\ ]+/cf230705/image/[^"\\\\ ]+\\.jpg'));
    items.push({ id: host.dramaId(id), source: host.source, sourceId: id, title: title, cover: coverMatch ? coverMatch[0] : '' });
  }
  return items;
}
exports.categories = async function () { return [{ id: '', name: '全部' }]; };
exports.catalogPage = async function (page, category) {
  if (page > 1) return { items: [], hasMore: false };
  return { items: parseCards(fetchText(SITE + '/')), hasMore: false };
};
exports.search = async function (page, query) {
  var q = String(query || '').trim();
  if (!q || page > 1) return { items: [], hasMore: false };
  return { items: parseCards(fetchText(SITE + '/')).filter(function (x) { return x.title.indexOf(q) >= 0; }), hasMore: false };
};
exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^[a-zA-Z0-9_-]+$/.test(id)) throw new Error('剧集标识无效');
  var html = unescapeRsc(fetchText(SITE + '/videos/' + id));
  var block = html.match(/initialVideo[\\"':\s]*\{([\s\S]*?)\}\s*,\s*"id"/);
  var seg = block ? block[0] : html;
  var titleMatch = seg.match(/"title":"([^"]+)"/);
  var title = titleMatch ? titleMatch[1] : id;
  var coverMatch = seg.match(/"cover":"([^"]+)"/);
  var episodes = [];
  var ePattern = /"chapterId":"([^"]+)","episode":(\d+)[^}]*?"url":"([^"]+)"/g;
  var m;
  while ((m = ePattern.exec(seg)) !== null) {
    episodes.push({ chapterId: m[1], number: Number(m[2]), url: m[3] });
  }
  var chapters = episodes.map(function (e, i) {
    return { id: host.chapterId(id, e.chapterId), source: host.source, title: '第' + e.number + '集', currentEpisode: e.number, url: e.url };
  });
  var drama = { id: host.dramaId(id), source: host.source, sourceId: id, title: title, cover: coverMatch ? (coverMatch[1].indexOf('http') === 0 ? coverMatch[1] : 'https://zzzznnn.lkkwip.cn/' + coverMatch[1]) : '' };
  if (chapters.length) drama.totalEpisode = chapters.length;
  return { drama: drama, chapters: chapters };
};
exports.resolveMedia = async function (task) {
  var chapter = (task && task.chapter) || {};
  var url = String(chapter.url || '');
  if (!url) {
    var drama = (task && task.drama) || {};
    var sourceId = String(drama.sourceId || '');
    if (!sourceId) { var id0 = String(drama.id || task.dramaId || ''); sourceId = id0.indexOf(':') >= 0 ? id0.split(':')[1] : id0; }
    if (sourceId) {
      var html = unescapeRsc(fetchText(SITE + '/videos/' + sourceId));
      var m = html.match(/"chapterId":"[^"]+","episode":(\d+)[^}]*?"url":"([^"]+)"/);
      if (m) url = m[2];
    }
  }
  if (!url) throw new Error('缺少播放参数');
  if (/^https?:/.test(url)) return { url: url, referer: SITE + '/' };
  return { url: SITE + '/api/app/vid/h5/m3u8/' + url.replace(/^\/+/, ''), referer: SITE + '/' };
};
