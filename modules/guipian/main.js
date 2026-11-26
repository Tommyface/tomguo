'use strict';
// 鬼片网（guipianwu.com）：站点搜索已停用，用 RSS 最新条目做标题匹配。
// 必须用 Android 移动 UA，桌面 UA 返回 403。
// 分类 /list/<id>.html，分页 /list/<id>_<page>.html
// 详情 /nv/<id>.html，播放 /play/<id>-<line>-<ep>.html，直链在 var now="..."

var SITE = 'https://guipianwu.com';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';
var PAGE_SIZE = 24;

var CATEGORIES = [
  { id: '3', name: '恐怖片' }, { id: '6', name: '大陆鬼片' },
  { id: '7', name: '日韩鬼片' }, { id: '8', name: '林正英鬼片' },
  { id: '9', name: '港台鬼片' }, { id: '10', name: '泰国鬼片' },
  { id: '11', name: '欧美鬼片' }, { id: '31', name: '剧情片' },
  { id: '12', name: '国产剧' }, { id: '13', name: '美剧' },
  { id: '14', name: '韩剧' }, { id: '15', name: '日剧' },
  { id: '16', name: '泰剧' }, { id: '17', name: '港台剧' },
  { id: '18', name: '其他剧' }, { id: '23', name: '日韩动漫' },
  { id: '24', name: '国产动漫' }, { id: '25', name: '欧美动漫' },
  { id: '26', name: '港台动漫' },
];

function fetchText(path) {
  var url = String(path || '');
  if (url.indexOf('http') !== 0) url = SITE + url;
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

function stripTags(text) {
  return String(text || '').replace(/<[^>]+>/g, '').trim();
}

// 只取主列表容器，避开顶部轮播的重复卡片。
function parseCards(html) {
  var segment = html;
  var listMatch = html.match(/<ul class="row video-list video-film-list[^"]*">([\s\S]*?)<\/ul>/i);
  if (listMatch) segment = listMatch[1];
  var items = [];
  var seen = {};
  var pattern = /<a class="video-link" href="\/nv\/(\d+)\.html"[^>]*title="([^"]*)"([\s\S]*?)<\/a>/gi;
  var match;
  while ((match = pattern.exec(segment)) !== null) {
    var id = match[1];
    if (seen[id]) continue;
    seen[id] = true;
    var coverMatch = match[3].match(/data-(?:original|background)="([^"]+)"/i);
    var remarkMatch = match[3].match(/<div class="video-duration">([\s\S]*?)<\/div>/i);
    items.push({
      id: host.dramaId(id),
      source: host.source,
      sourceId: id,
      title: match[2],
      cover: coverMatch ? (coverMatch[1].indexOf('http') === 0 ? coverMatch[1] : SITE + coverMatch[1]) : '',
      remark: remarkMatch ? stripTags(remarkMatch[1]) : '',
    });
  }
  return items;
}

function listHref(category, page) {
  var cat = String(category || '').trim() || CATEGORIES[0].id;
  if (page <= 1) return '/list/' + cat + '.html';
  return '/list/' + cat + '_' + page + '.html';
}

exports.categories = async function () {
  return CATEGORIES;
};

exports.catalogPage = async function (page, category) {
  var html = fetchText(listHref(category, page));
  var items = parseCards(html);
  if (items.length === 0) return { items: [], hasMore: false };
  // 翻页链接：/list/3_2.html 或「下一页」按钮
  var hasMore = new RegExp('/list/' + String(category || CATEGORIES[0].id) + '_' + (page + 1) + '\\.html').test(html)
    || /下一页/.test(html);
  return { items: items, hasMore: hasMore };
};

// 站点搜索已停用：在 RSS 最新条目里做标题匹配，命中范围限于最新一批。
exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword || page > 1) return { items: [], hasMore: false };
  var rss = fetchText('/xml/rss.xml');
  var items = [];
  var seen = {};
  var pattern = /<item>([\s\S]*?)<\/item>/gi;
  var match;
  while ((match = pattern.exec(rss)) !== null) {
    var block = match[1];
    var titleMatch = block.match(/<title>\s*(?:<!\[CDATA\[)?([\s\S]*?)(?:\]\]>)?\s*<\/title>/i);
    var linkMatch = block.match(/\/nv\/(\d+)\.html/);
    if (!titleMatch || !linkMatch) continue;
    var title = stripTags(titleMatch[1]);
    var id = linkMatch[1];
    if (seen[id] || keyword.toLowerCase().indexOf(title.toLowerCase()) < 0 && title.toLowerCase().indexOf(keyword.toLowerCase()) < 0) continue;
    seen[id] = true;
    items.push({ id: host.dramaId(id), source: host.source, sourceId: id, title: title });
  }
  return { items: items, hasMore: false };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^\d+$/.test(id)) throw new Error('剧集标识无效');
  var html = fetchText('/nv/' + id + '.html');
  var titleMatch = html.match(/<h1 class="media-title[^"]*">([\s\S]*?)<\/h1>/i)
    || html.match(/<title>([\s\S]*?)<\/title>/i);
  var title = titleMatch ? stripTags(titleMatch[1]).replace(/\s*-\s*鬼片网.*$/, '') : ('鬼片网 ' + id);
  var coverMatch = html.match(/<div class="detail-img"><img[^>]+src="([^"]+)"/i)
    || html.match(/<div class="detail-cover[^"]*"[^>]*data-original="([^"]+)"/i);
  var descMatch = html.match(/简介：([\s\S]*?)<\/li>/i);

  // 每个线路一个 ewave-tab（data-target="#ewave-playlist-N"），
  // 剧集链接在 <li><a title='...' href='/play/id-line-ep.html'。
  var chapters = [];
  var seen = {};
  var tabPattern = /<li class="swiper-slide ewave-tab[^"]*"\s+data-target="#ewave-playlist-(\d+)">([^<]+)<\/li>/gi;
  var tabMatch;
  var tabs = [];
  while ((tabMatch = tabPattern.exec(html)) !== null) {
    tabs.push({ line: tabMatch[1], name: stripTags(tabMatch[2]) });
  }
  if (tabs.length === 0) tabs.push({ line: '1', name: '默认' });

  for (var t = 0; t < tabs.length; t++) {
    var line = tabs[t].line;
    var linePattern = new RegExp("<li><a title='([^']*)' href='/play/" + id + "-" + line + "-(\\d+)\\.html'", 'gi');
    var match;
    while ((match = linePattern.exec(html)) !== null) {
      var ep = match[2];
      var key = line + ':' + ep;
      if (seen[key]) continue;
      seen[key] = true;
      chapters.push({
        id: host.chapterId(id, key),
        source: host.source,
        title: stripTags(match[1]) || ('第' + (chapters.length + 1) + '集'),
        currentEpisode: chapters.length + 1,
        line: line,
        ep: ep,
        lineName: tabs[t].name,
      });
    }
    if (chapters.length > 0) break; // 只取第一个有剧集的线路，避免重复
  }

  var drama = {
    id: host.dramaId(id),
    source: host.source,
    sourceId: id,
    title: title,
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
  var line = String(chapter.line || '1');
  var ep = String(chapter.ep || '0');
  if (!ep) {
    var chapterId = String(chapter.id || '');
    var parts = chapterId.split(':');
    if (parts.length >= 3) { line = parts[2]; ep = parts[3] || '0'; }
  }
  if (!sourceId) throw new Error('缺少播放参数');
  var playUrl = '/play/' + sourceId + '-' + line + '-' + ep + '.html';
  var html = fetchText(playUrl);
  var now = html.match(/var\s+now\s*=\s*["']([^"']+)["']/i);
  var any = html.match(/https?:\/\/[^"'\s<>\\]+\.(?:m3u8|mp4)(?:\?[^"'\s<>\\]*)?/i);
  var media = now ? now[1] : (any ? any[0] : '');
  if (!media) throw new Error('该集暂时取不到播放地址');
  return { url: media, referer: SITE + playUrl };
};
