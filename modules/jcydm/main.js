'use strict';
// 囧次元（jcydm1.com）：标准 MacCMS v10。
// 列表 /index.php/vod/type/id/21.html（分页 .../page/N.html）
// 搜索 /index.php/vod/search/wd/<kw>.html
// 详情 /index.php/vod/detail/id/<id>.html
// 播放 /index.php/vod/play/id/<id>/sid/<sid>/nid/<nid>.html，地址在页面内嵌 JSON 的 "url" 字段。
//
// 导出用 async（api 2 形态），宿主 invoker 会等待 Promise；manifest api 仍为 1，
// 因此同一个模块在旧客户端上也能加载（旧 invoker 会因返回 Promise 报错，故升版本时再切）。

var SITE = 'https://www.jcydm1.com';
var UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';

var CATEGORIES = [
  { id: '21', name: '日漫' },
  { id: '23', name: '国漫' },
];

function fetchText(url, referer) {
  var response = host.fetch({
    url: url,
    method: 'GET',
    headers: { 'User-Agent': UA, Referer: referer || SITE + '/' },
    timeoutMs: 12000,
  });
  if (!response || response.status < 200 || response.status >= 400) {
    throw new Error('请求失败：' + (response ? response.status : 'no response'));
  }
  // 宿主同时返回 body（文本）与 bodyBase64；优先用文本，避免把文本覆盖成字节数组。
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}

// 列表页每个条目：<a href=".../detail/id/<id>.html" title="<标题>" class="module-poster-item ...">
// 封面在同一块的 data-original。
function parseCards(html) {
  var items = [];
  var seen = {};
  var pattern = /<a href="\/index\.php\/vod\/detail\/id\/(\d+)\.html" title="([^"]*)"[^>]*class="module-poster-item/g;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var id = match[1];
    if (seen[id]) continue;
    seen[id] = true;
    var tail = html.slice(match.index, match.index + 900);
    var cover = '';
    var coverMatch = tail.match(/data-original="([^"]+)"/);
    if (coverMatch) cover = coverMatch[1];
    var note = '';
    var noteMatch = tail.match(/module-item-note">([^<]*)</);
    if (noteMatch) note = noteMatch[1];
    items.push({
      id: host.dramaId(id),
      source: host.source,
      sourceId: id,
      title: match[2],
      cover: cover,
      remark: note,
    });
  }
  return items;
}

// 搜索页的卡片结构不同：<a href=".../detail/id/<id>.html" class="module-card-item-poster">，
// 标题在 <img alt="..."> 里而不是 title 属性。
function parseSearchCards(html) {
  var items = [];
  var seen = {};
  var pattern = /<a href="\/index\.php\/vod\/detail\/id\/(\d+)\.html" class="module-card-item-poster">/g;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var id = match[1];
    if (seen[id]) continue;
    seen[id] = true;
    var tail = html.slice(match.index, match.index + 1200);
    var title = '';
    var titleMatch = tail.match(/<img[^>]*alt="([^"]*)"/);
    if (titleMatch) title = titleMatch[1].trim();
    var cover = '';
    var coverMatch = tail.match(/data-original="([^"]+)"/);
    if (coverMatch) cover = coverMatch[1];
    if (!title) continue;
    items.push({
      id: host.dramaId(id),
      source: host.source,
      sourceId: id,
      title: title,
      cover: cover,
    });
  }
  return items;
}

function pageHref(category, page) {
  var base = SITE + '/index.php/vod/type/id/' + category + '.html';
  if (page <= 1) return base;
  return SITE + '/index.php/vod/type/id/' + category + '/page/' + page + '.html';
}

exports.categories = async function () {
  return CATEGORIES;
};

exports.catalogPage = async function (page, category) {
  var cat = String(category || '').trim() || CATEGORIES[0].id;
  var html = fetchText(pageHref(cat, page));
  var items = parseCards(html);
  // 分页存在性：页面里出现比当前页更大的 page/ 链接就还有下一页。
  var hasMore = false;
  var nextPattern = new RegExp('/page/' + (page + 1) + '\\.html');
  if (nextPattern.test(html)) hasMore = true;
  if (items.length === 0) return { items: [], hasMore: false };
  return { items: items, hasMore: hasMore };
};

exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword) return { items: [], hasMore: false };
  var base = SITE + '/index.php/vod/search/wd/' + encodeURIComponent(keyword) + '.html';
  var url = page > 1 ? SITE + '/index.php/vod/search/wd/' + encodeURIComponent(keyword) + '/page/' + page + '.html' : base;
  var html = fetchText(url);
  var items = parseSearchCards(html);
  if (items.length === 0) items = parseCards(html);
  var hasMore = new RegExp('/page/' + (page + 1) + '\\.html').test(html);
  return { items: items, hasMore: hasMore && items.length > 0 };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^\d+$/.test(id)) throw new Error('剧集标识无效');
  var html = fetchText(SITE + '/index.php/vod/detail/id/' + id + '.html');
  var titleMatch = html.match(/<h1[^>]*>([^<]*)<\/h1>/);
  var title = titleMatch ? titleMatch[1].trim() : '';
  var coverMatch = html.match(/data-original="([^"]+)"/);
  var descMatch = html.match(/<div class="module-info-introduction-content"[^>]*>[\s\S]*?<p>([\s\S]*?)<\/p>/);

  // 剧集：/play/id/<id>/sid/<sid>/nid/<nid>.html
  var chapters = [];
  var seen = {};
  // 剧集链接带属性（class / title），标题可能是属性后的文本或 title 里的「播放X第N集」。
  var chapterPattern = /href="\/index\.php\/vod\/play\/id\/\d+\/sid\/(\d+)\/nid\/(\d+)\.html"[^>]*>([^<]*)</g;
  var match;
  while ((match = chapterPattern.exec(html)) !== null) {
    var nid = match[2];
    if (seen[nid]) continue;
    seen[nid] = true;
    var label = (match[3] || '').trim();
    if (!label) {
      var attrMatch = html.slice(match.index, match.index + 400).match(/title="播放[^"]*?第(\d+)集"/);
      if (attrMatch) label = '第' + attrMatch[1] + '集';
    }
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
    title: title || ('囧次元 ' + id),
    cover: coverMatch ? coverMatch[1] : '',
  };
  if (descMatch) drama.description = descMatch[1].replace(/<[^>]+>/g, '').trim();
  if (chapters.length) drama.totalEpisode = chapters.length;
  return { drama: drama, chapters: chapters };
};

exports.resolveMedia = async function (task) {
  var chapter = (task && task.chapter) || {};
  var chapterId = String(chapter.id || '');
  var parts = chapterId.split(':');
  var nid = parts.length >= 3 ? parts[2] : '';
  var dramaId = String((task && task.dramaId) || '');
  var sourceId = dramaId.indexOf(':') >= 0 ? dramaId.split(':')[1] : dramaId;
  var sid = String(chapter.sid || '1');
  if (!sourceId || !nid) throw new Error('缺少播放参数');
  var url = SITE + '/index.php/vod/play/id/' + sourceId + '/sid/' + sid + '/nid/' + nid + '.html';
  var html = fetchText(url);
  var playMatch = html.match(/"url":"([^"]+\.m3u8[^"]*)"/) || html.match(/"(https?:\\?\/\\?\/[^"]+\.m3u8[^"]*)"/);
  if (!playMatch) throw new Error('该集暂时取不到播放地址');
  var media = playMatch[1].replace(/\\\//g, '/');
  return { url: media, referer: url };
};
