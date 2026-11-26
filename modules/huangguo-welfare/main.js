'use strict';
// 黄果福利：CloudFront 门户站（d24dalcptr65cw.cloudfront.net）
// 首页 <a class="poster-card" href="/detail/<id>/"> 卡片，封面在 data-src。
// 详情 /detail/<id>/，分集 <a class="detail-episode" data-chapter-id="...">，播放 /play/<id>/<chapterId>/。

var SITE = 'https://d24dalcptr65cw.cloudfront.net';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';

function fetchText(url) {
  var response = host.fetch({ url: url, method: 'GET', headers: { 'User-Agent': UA, Referer: SITE + '/' }, timeoutMs: 20000 });
  if (!response || response.status < 200 || response.status >= 400) throw new Error('请求失败：' + (response ? response.status : 'no response'));
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}
function stripTags(s) { return String(s || '').replace(/<[^>]*>/g, '').trim(); }
function parseCards(html) {
  var items = [], seen = {};
  var pattern = /<a\b[^>]*class="[^"]*poster-card[^"]*"[^>]*href="\/detail\/([a-zA-Z0-9_-]{2,64})\/"[^>]*>([\s\S]*?)<\/a>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var id = match[1];
    if (seen[id]) continue;
    seen[id] = true;
    var block = match[2];
    var img = block.match(/<img\b[^>]*>/i);
    var cover = '';
    if (img) {
      var cm = img[0].match(/\b(?:data-src|data-original|src)="([^"]+)"/i);
      if (cm && cm[1].indexOf('placeholder') < 0) cover = /^https?:/.test(cm[1]) ? cm[1] : SITE + cm[1];
    }
    var title = ((img && (img[0].match(/alt="([^"]+)"/) || [])[1]) || '').trim();
    if (!title) title = stripTags(block.replace(/<img[^>]*>/gi, '')).split(/\s{2,}/)[0];
    if (!title) continue;
    items.push({ id: host.dramaId(id), source: host.source, sourceId: id, title: title, cover: cover });
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
  var all = parseCards(fetchText(SITE + '/'));
  return { items: all.filter(function (x) { return x.title.indexOf(q) >= 0; }), hasMore: false };
};
exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^[a-zA-Z0-9_-]{2,64}$/.test(id)) throw new Error('剧集标识无效');
  var html = fetchText(SITE + '/detail/' + id + '/');
  var titleMatch = html.match(/<h1\b[^>]*>([\s\S]*?)<\/h1>/i) || html.match(/<title>([\s\S]*?)<\/title>/i);
  var title = titleMatch ? stripTags(titleMatch[1]) : id;
  var coverMatch = html.match(/\bdata-src="(https?:\/\/[^"]+)"/);
  var chapters = [], seen = {};
  var pattern = /<a\b[^>]*class="[^"]*detail-episode[^"]*"[^>]*>([\s\S]*?)<\/a>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var tag = match[0];
    var chapterId = (tag.match(/data-chapter-id="([^"]+)"/) || [])[1] || '';
    if (!chapterId || seen[chapterId]) continue;
    seen[chapterId] = true;
    var ep = Number((tag.match(/data-ep="(\d+)"/) || [])[1] || chapters.length + 1);
    chapters.push({ id: host.chapterId(id, chapterId), source: host.source, title: '第' + ep + '集', currentEpisode: ep, chapterId: chapterId });
  }
  var drama = { id: host.dramaId(id), source: host.source, sourceId: id, title: title, cover: coverMatch ? coverMatch[1] : '' };
  if (chapters.length) drama.totalEpisode = chapters.length;
  return { drama: drama, chapters: chapters };
};
exports.resolveMedia = async function (task) {
  var chapter = (task && task.chapter) || {};
  var drama = (task && task.drama) || {};
  var sourceId = String(drama.sourceId || '');
  if (!sourceId) { var id0 = String(drama.id || task.dramaId || ''); sourceId = id0.indexOf(':') >= 0 ? id0.split(':')[1] : id0; }
  var chapterId = String(chapter.chapterId || '');
  if (!chapterId) { var parts = String(chapter.id || '').split(':'); if (parts.length >= 3) chapterId = parts[2]; }
  if (!sourceId || !chapterId) throw new Error('缺少播放参数');
  var url = SITE + '/play/' + sourceId + '/' + chapterId + '/';
  var html = fetchText(url);
  if (/data-need-unlock="1"/.test(html) && !/data-main-url="([^"]+)"/.test(html)) throw new Error('该集需要解锁或金币，未取得正片地址');
  var m = html.match(/data-url="([^"]+)"/) || html.match(/data-main-url="([^"]+)"/) || html.match(/data-preview-url="([^"]+)"/);
  var any = html.match(/https?:\/\/[^"'\s<>]+\.m3u8[^"'\s<>]*/);
  var media = m && m[1] ? m[1] : (any ? any[0] : '');
  if (!media) throw new Error('该集暂时取不到播放地址');
  if (media.indexOf('//') === 0) media = 'https:' + media;
  if (media.indexOf('http') !== 0) media = SITE + '/' + media.replace(/^\/+/, '');
  return { url: media, referer: url };
};
