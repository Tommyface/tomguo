'use strict';
// 花魁短剧（huakui.net）：Next.js 站。
// 首页 / 卡片取标题与链接，封面从 RSC 数据（self.__next_f.push）里的 playName/cover 补。
// 详情 /drama/<slug>/，分集 /drama/<slug>/<章节ID>/。
// 播放页 videoUrl 为相对路径，需改写为 https://huakui.net/h5/m3u8/<path>。

var SITE = 'https://huakui.net';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';

function fetchText(url) {
  var response = host.fetch({
    url: url, method: 'GET',
    headers: { 'User-Agent': UA, Referer: SITE + '/' },
    timeoutMs: 15000,
  });
  if (!response || response.status < 200 || response.status >= 400) {
    throw new Error('请求失败：' + (response ? response.status : 'no response'));
  }
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}

function stripTags(s) {
  return String(s || '').replace(/<[^>]*>/g, '').replace(/&quot;/g, '"').trim();
}

// RSC 数据里的封面索引：playName → cover，用来补卡片上的占位封面。
function rscCoverMap(html) {
  var map = {};
  var pattern = /playName\\?":\\?"([^"\\]+)[^}]{0,400}?cover\\?":\\?"([^"\\]+)/g;
  var match;
  while ((match = pattern.exec(html)) !== null) map[match[1]] = match[2];
  return map;
}

function parseCards(html) {
  var covers = rscCoverMap(html);
  var items = [];
  var seen = {};
  var pattern = /<article class="hk-drama-card">[\s\S]*?href="(\/drama\/([a-z0-9]{4,40})\/)"[\s\S]*?<h3 class="hk-drama-card__title">\s*<a[^>]*>([^<]*)<\/a>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var id = match[2];
    if (seen[id]) continue;
    seen[id] = true;
    var title = stripTags(match[3]);
    var cover = covers[title] || '';
    if (!cover) {
      var styleMatch = html.slice(match.index, match.index + 500).match(/--cover:url\(&quot;([^&]+)&quot;\)/);
      if (styleMatch && styleMatch[1].indexOf('placeholder') < 0) cover = styleMatch[1];
    }
    items.push({
      id: host.dramaId(id),
      source: host.source,
      sourceId: id,
      title: title,
      cover: cover.indexOf('http') === 0 ? cover : (cover ? SITE + cover : ''),
    });
  }
  return items;
}

exports.categories = async function () {
  return [{ id: '', name: '全部' }];
};

exports.catalogPage = async function (page, category) {
  var html = fetchText(SITE + '/');
  var items = parseCards(html);
  // 站点没有真正的分页入口，首页是编排列表；后续页返回空，避免无限重试。
  if (page > 1) return { items: [], hasMore: false };
  return { items: items, hasMore: false };
};

exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword || page > 1) return { items: [], hasMore: false };
  // 站点未提供公开搜索接口；在首页条目里做本地标题匹配。
  var all = parseCards(fetchText(SITE + '/'));
  var items = [];
  for (var i = 0; i < all.length; i++) {
    if (all[i].title.indexOf(keyword) >= 0) items.push(all[i]);
  }
  return { items: items, hasMore: false };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^[a-z0-9]{4,40}$/.test(id)) throw new Error('剧集标识无效');
  var html = fetchText(SITE + '/drama/' + id + '/');
  var titleMatch = html.match(/<h1 class="hk-playback-meta__title">([^<]*)<\/h1>/);
  var title = titleMatch ? stripTags(titleMatch[1]) : id;
  var coverMatch = html.match(/--cover:url\(&quot;([^&]+)&quot;\)/);
  var descMatch = html.match(/hk-playback-meta__description[^>]*>([\s\S]*?)<\/p>/i);

  var chapters = [];
  var seen = {};
  var pattern = /href="\/drama\/[a-z0-9]{4,40}\/(\d{1,8})\/"[^>]*>([\s\S]*?)<\/a>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var chapterId = match[1];
    if (seen[chapterId]) continue;
    seen[chapterId] = true;
    var label = stripTags(match[2]) || ('第' + (chapters.length + 1) + '集');
    chapters.push({
      id: host.chapterId(id, chapterId),
      source: host.source,
      title: label,
      currentEpisode: chapters.length + 1,
      chapterId: chapterId,
      pageUrl: SITE + '/drama/' + id + '/' + chapterId + '/',
    });
  }

  var drama = {
    id: host.dramaId(id), source: host.source, sourceId: id, title: title,
    cover: coverMatch ? (coverMatch[1].indexOf('http') === 0 ? coverMatch[1] : SITE + coverMatch[1]) : '',
  };
  if (descMatch) drama.description = stripTags(descMatch[1]);
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
  var pageUrl = String(chapter.pageUrl || '');
  var chapterId = String(chapter.chapterId || '');
  if (!chapterId) {
    var cid = String(chapter.id || '');
    var parts = cid.split(':');
    if (parts.length >= 3) chapterId = parts[2];
  }
  if (!sourceId || !chapterId) throw new Error('缺少播放参数');
  if (!pageUrl) pageUrl = SITE + '/drama/' + sourceId + '/' + chapterId + '/';
  var html = fetchText(pageUrl);
  if (/playback-player--locked|data-can-watch="0"/.test(html)) {
    throw new Error('该集需要解锁或会员权限，未取得正片地址');
  }
  var plain = html.replace(/\\"/g, '"').replace(/\\\//g, '/');
  var match = plain.match(/videoUrl\\?":\\?"([^"\\]+)\\?"/);
  if (!match) throw new Error('该集暂时取不到播放地址');
  var video = match[1];
  if (video.indexOf('http') === 0) return { url: video, referer: pageUrl };
  var path = video.replace(/^\/+/, '');
  if (path.indexOf('h5/m3u8/') !== 0) path = 'h5/m3u8/' + path;
  return { url: SITE + '/' + path, referer: pageUrl };
};
