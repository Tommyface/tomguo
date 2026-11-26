'use strict';
// 黄果精选：CloudFront 门户站（d2teten6pcf3ig.cloudfront.net）
// 首页 <article class="card"> 卡片：/play/<id>/，封面在 data-src。
// 详情 /play/<id>/，分集 /play/<id>/<videoId>/，播放器块 data-main-url / data-preview-url。

var SITE = 'https://d2teten6pcf3ig.cloudfront.net';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';

function fetchText(url) {
  var response = host.fetch({ url: url, method: 'GET', headers: { 'User-Agent': UA, Referer: SITE + '/' }, timeoutMs: 20000 });
  if (!response || response.status < 200 || response.status >= 400) throw new Error('请求失败：' + (response ? response.status : 'no response'));
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}
function stripTags(s) { return String(s || '').replace(/<[^>]*>/g, '').trim(); }
function coverFrom(block) {
  var m = block.match(/\b(?:data-src|data-original|data-lazy-src|src)="([^"]+)"/i);
  if (m && m[1].indexOf('placeholder') < 0) return /^https?:/.test(m[1]) ? m[1] : SITE + m[1];
  var css = block.match(/--cover:\s*url\(['"]?([^'")]+)/i);
  if (css && css[1].indexOf('placeholder') < 0) return /^https?:/.test(css[1]) ? css[1] : SITE + css[1];
  return '';
}
function parseCards(html) {
  var items = [], seen = {};
  var pattern = /<article\b[^>]*class="[^"]*card[^"]*"[^>]*>([\s\S]*?)<\/article>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var block = match[1];
    var link = block.match(/href="\/play\/([a-zA-Z0-9_-]{2,64})\/"/);
    if (!link) continue;
    var id = link[1];
    if (seen[id]) continue;
    seen[id] = true;
    var titleMatch = block.match(/<h3\b[^>]*class="[^"]*card__title[^"]*"[^>]*>\s*<a\b[^>]*>([\s\S]*?)<\/a>/i);
    var title = titleMatch ? stripTags(titleMatch[1]) : '';
    if (!title) continue;
    items.push({ id: host.dramaId(id), source: host.source, sourceId: id, title: title, cover: coverFrom(block) });
  }
  return items;
}
exports.categories = async function () { return [{ id: '', name: '全部' }]; };
exports.catalogPage = async function (page, category) {
  if (page > 1) return { items: [], hasMore: false };
  var items = parseCards(fetchText(SITE + '/'));
  return { items: items, hasMore: false };
};
exports.search = async function (page, query) {
  var q = String(query || '').trim();
  if (!q || page > 1) return { items: [], hasMore: false };
  var all = parseCards(fetchText(SITE + '/'));
  return { items: all.filter(function (item) { return item.title.indexOf(q) >= 0; }), hasMore: false };
};
exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^[a-zA-Z0-9_-]{2,64}$/.test(id)) throw new Error('剧集标识无效');
  var html = fetchText(SITE + '/play/' + id + '/');
  var titleMatch = html.match(/<h1\b[^>]*>([\s\S]*?)<\/h1>/i);
  var title = titleMatch ? stripTags(titleMatch[1]) : id;
  var chapters = [], seen = {};
  var pattern = /<a\b[^>]*class="[^"]*play-ep[^"]*"[^>]*>([\s\S]*?)<\/a>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var tag = match[0];
    var videoId = (tag.match(/data-target-video-id="([^"]+)"/) || [])[1] || '';
    if (!videoId || seen[videoId]) continue;
    seen[videoId] = true;
    var ep = Number((tag.match(/data-target-episode="(\d+)"/) || [])[1] || chapters.length + 1);
    chapters.push({ id: host.chapterId(id, videoId), source: host.source, title: '第' + ep + '集', currentEpisode: ep, videoId: videoId });
  }
  var drama = { id: host.dramaId(id), source: host.source, sourceId: id, title: title, cover: coverFrom(html) };
  if (chapters.length) drama.totalEpisode = chapters.length;
  return { drama: drama, chapters: chapters };
};
exports.resolveMedia = async function (task) {
  var chapter = (task && task.chapter) || {};
  var drama = (task && task.drama) || {};
  var sourceId = String(drama.sourceId || '');
  if (!sourceId) {
    var id0 = String(drama.id || task.dramaId || '');
    sourceId = id0.indexOf(':') >= 0 ? id0.split(':')[1] : id0;
  }
  var videoId = String(chapter.videoId || '');
  if (!videoId) { var parts = String(chapter.id || '').split(':'); if (parts.length >= 3) videoId = parts[2]; }
  if (!sourceId || !videoId) throw new Error('缺少播放参数');
  var url = SITE + '/play/' + sourceId + '/' + videoId + '/';
  var html = fetchText(url);
  if (/"data-can-watch="0"/.test(html) || /data-can-watch="0"/.test(html)) {
    var main = (html.match(/data-main-url="([^"]*)"/) || [])[1] || '';
    var preview = (html.match(/data-preview-url="([^"]*)"/) || [])[1] || '';
    if (!main && !preview) throw new Error('该集需要解锁或会员权限，未取得正片地址');
  }
  var m = html.match(/data-main-url="([^"]+)"/) || html.match(/data-preview-url="([^"]+)"/);
  var any = html.match(/https?:\/\/[^"'\s<>]+\.m3u8[^"'\s<>]*/);
  var media = m && m[1] ? m[1] : (any ? any[0] : '');
  if (!media) throw new Error('该集暂时取不到播放地址');
  if (media.indexOf('//') === 0) media = 'https:' + media;
  if (media.indexOf('http') !== 0) media = SITE + '/' + media.replace(/^\/+/, '');
  return { url: media, referer: url };
};
