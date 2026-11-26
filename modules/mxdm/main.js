'use strict';
// MX动漫（dcc3.com）：MacCMS 变体。
// 列表 /type/<分类>/，第 N 页是 /type/<分类>/N/（**结尾带斜杠**，/page/N/ 会 404）
// 搜索 /search/?wd=<kw>（只有一页，站点侧不认 page）
// 详情 /detail/<id>/，剧集 /play/<id>-<sid>-<nid>/
// 播放页直接内嵌 m3u8 直链，无需签名。

var SITE = 'https://www.dcc3.com';
var UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';

var CATEGORIES = [
  { id: 'riman', name: '日本动漫' },
  { id: 'guoman', name: '国产动漫' },
  { id: 'oman', name: '欧美动漫' },
  { id: 'dmfilm', name: '动漫电影' },
];

function fetchText(url) {
  var response = host.fetch({
    url: url,
    method: 'GET',
    headers: { 'User-Agent': UA, Referer: SITE + '/' },
    timeoutMs: 12000,
  });
  if (!response || response.status < 200 || response.status >= 400) {
    throw new Error('请求失败：' + (response ? response.status : 'no response'));
  }
  // 宿主同时给 body（文本）与 bodyBase64，优先用文本。
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}

// 列表与搜索的卡片 class 不同（列表是裸 <a>，搜索是 <a class="cover">），
// 但都是 href="/detail/<id>/" title="<标题>" 开头，后面跟 img data-original。
function parseCards(html) {
  var items = [];
  var seen = {};
  var pattern = /href="\/detail\/(\d+)\/?"[^>]*title="([^"]*)"/g;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var id = match[1];
    if (seen[id]) continue;
    seen[id] = true;
    var tail = html.slice(match.index, match.index + 900);
    var cover = '';
    var coverMatch = tail.match(/data-original="([^"]+)"/);
    if (coverMatch) cover = coverMatch[1];
    var remark = '';
    var remarkMatch = tail.match(/<p>(更新至[^<]*)<\/p>/);
    if (remarkMatch) remark = remarkMatch[1];
    items.push({
      id: host.dramaId(id),
      source: host.source,
      sourceId: id,
      title: match[2],
      cover: cover,
      remark: remark,
    });
  }
  return items;
}

function listHref(category, page) {
  var base = SITE + '/type/' + category + '/';
  if (page <= 1) return base;
  return SITE + '/type/' + category + '/' + page + '/';
}

exports.categories = async function () {
  return CATEGORIES;
};

exports.catalogPage = async function (page, category) {
  var cat = String(category || '').trim() || CATEGORIES[0].id;
  var html = fetchText(listHref(cat, page));
  var items = parseCards(html);
  if (items.length === 0) return { items: [], hasMore: false };
  // 翻页链接形如 /type/riman/2/，看有没有比当前页大的。
  var hasMore = new RegExp('/type/' + cat + '/' + (page + 1) + '/').test(html);
  return { items: items, hasMore: hasMore };
};

exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword) return { items: [], hasMore: false };
  // 站点搜索不认页码，第 2 页起直接返回空。
  if (page > 1) return { items: [], hasMore: false };
  var html = fetchText(SITE + '/search/?wd=' + encodeURIComponent(keyword));
  var items = parseCards(html);
  return { items: items, hasMore: false };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^\d+$/.test(id)) throw new Error('剧集标识无效');
  var html = fetchText(SITE + '/detail/' + id + '/');

  // 页面没有 <h1>，标题在 <title> 里，格式「片名-片名在线观看-分类-MX动漫」。
  var title = '';
  var titleMatch = html.match(/<title>([^<]*)<\/title>/);
  if (titleMatch) {
    title = titleMatch[1].split('-')[0].trim();
  }
  var coverMatch = html.match(/data-original="([^"]+)"/);

  // 剧集 /play/<id>-<sid>-<nid>/，多线路时 sid 不同；同一 nid 只取第一条。
  var chapters = [];
  var seen = {};
  var pattern = /href="\/play\/\d+-(\d+)-(\d+)\/?"[^>]*>([^<]*)</g;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var nid = match[2];
    if (seen[nid]) continue;
    seen[nid] = true;
    var label = (match[3] || '').trim();
    chapters.push({
      id: host.chapterId(id, nid),
      source: host.source,
      title: label || ('第' + (chapters.length + 1) + '集'),
      currentEpisode: chapters.length + 1,
      sid: match[1],
      nid: nid,
    });
  }

  var drama = {
    id: host.dramaId(id),
    source: host.source,
    sourceId: id,
    title: title || ('MX动漫 ' + id),
    cover: coverMatch ? coverMatch[1] : '',
  };
  if (chapters.length) drama.totalEpisode = chapters.length;
  return { drama: drama, chapters: chapters };
};

exports.resolveMedia = async function (task) {
  var chapter = (task && task.chapter) || {};
  var drama = (task && task.drama) || {};
  var sourceId = String(drama.sourceId || '');
  if (!sourceId) {
    var dramaId = String(drama.id || task.dramaId || '');
    sourceId = dramaId.indexOf(':') >= 0 ? dramaId.split(':')[1] : dramaId;
  }
  var nid = String(chapter.nid || '');
  var sid = String(chapter.sid || '1');
  if (!nid) {
    var chapterId = String(chapter.id || '');
    var parts = chapterId.split(':');
    nid = parts.length >= 3 ? parts[2] : '';
  }
  if (!sourceId || !nid) throw new Error('缺少播放参数');
  var playUrl = SITE + '/play/' + sourceId + '-' + sid + '-' + nid + '/';
  var html = fetchText(playUrl);
  var match = html.match(/(https?:\/\/[^"'\s]+\.m3u8[^"'\s]*)/);
  if (!match) throw new Error('该集暂时取不到播放地址');
  return { url: match[1], referer: playUrl };
};
