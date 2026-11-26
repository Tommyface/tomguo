'use strict';
// E站动漫（ezdmw.org）：无真正分页接口，首页是编排页，后续用 search_some 的偏移分页。
// 播放页含 player.ezdmw.com/danmuku iframe，再请求该 iframe 拿 <video src> 的 listres 地址。
// 注意 iframe URL 中的 title/quarterly 含空格：保留原样，只在最终请求前把空格编码。

var SITE = 'https://www.ezdmw.org';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';
var PAGE_SIZE = 10;

function fetchText(url, referer) {
  var response = host.fetch({
    url: url, method: 'GET',
    headers: { 'User-Agent': UA, Referer: referer || SITE + '/' },
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
  return String(s || '').replace(/<[^>]*>/g, '').replace(/&nbsp;/g, ' ').trim();
}

// 卡片标题：<a> 内的 <p> 文案，末尾可能有「更至12话」「【连载中】」元信息。
function cleanTitle(raw) {
  var text = stripTags(raw);
  text = text.replace(/^[\s\S]*?<!--[\s\S]*?-->/, '');
  text = text.replace(/[\s\u00a0]*(?:更至|更新至|更新到)[\s\u00a0:：]*\d+[\s\S]*$/, '');
  text = text.replace(/[\s\S]*?\d{4}年\d+月【[^】]*】/, '').trim();
  text = text.replace(/[\s\u00a0]*\d+天前[\s\S]*$/, '').trim();
  return text;
}

function parseCards(html) {
  var items = [];
  var seen = {};
  var pattern = /<a\s+href="(\/Index\/(?:bangumi|show|video|detail)\/\d+\.html)"[^>]*>([\s\S]*?)<\/a>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var href = match[1];
    var idMatch = href.match(/(\d+)\.html$/);
    if (!idMatch) continue;
    var id = idMatch[1];
    if (seen[id]) continue;
    seen[id] = true;
    var title = cleanTitle(match[2]);
    if (!title) continue;
    var coverMatch = match[2].match(/<img[^>]+(?:data-src|data-original|src)="([^"]+)"/i);
    var episodesMatch = match[2].match(/(?:更新到|更新至|更至)[\s\u00a0:：]*0*(\d+)/i);
    items.push({
      id: host.dramaId(id),
      source: host.source,
      sourceId: id,
      title: title,
      cover: coverMatch ? (coverMatch[1].indexOf('http') === 0 ? coverMatch[1] : SITE + coverMatch[1]) : '',
      totalEpisode: episodesMatch ? Number(episodesMatch[1]) : 0,
    });
  }
  return items;
}

function listURL(keyword, offset) {
  return SITE + '/Index/search_some.html?searchText=' + encodeURIComponent(keyword) + '&page=' + offset;
}

exports.categories = async function () {
  // 站点没有可用的一级分类；固定返回单类，让首页按站源浏览时有入口。
  return [{ id: '', name: '全部' }];
};

exports.catalogPage = async function (page, category) {
  var html;
  if (page <= 1) {
    html = fetchText(SITE + '/');
  } else {
    // 用「年」做通配词：站点搜索也匹配年份字段，每部都有年份，等价于全站浏览。
    html = fetchText(listURL('年', (page - 2) * PAGE_SIZE));
  }
  var items = parseCards(html);
  return { items: items, hasMore: items.length >= 10 || page === 1 };
};

exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword || page > 100000) return { items: [], hasMore: false };
  var html = fetchText(listURL(keyword, (page - 1) * PAGE_SIZE));
  var items = parseCards(html);
  return { items: items, hasMore: items.length >= PAGE_SIZE };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^\d+$/.test(id)) throw new Error('剧集标识无效');
  var html = fetchText(SITE + '/Index/bangumi/' + id + '.html');
  var titleMatch = html.match(/<title>([\s\S]*?)<\/title>/i);
  var title = titleMatch ? stripTags(titleMatch[1]).replace(/\s*[-|_]\s*E站.*$/, '').trim() : ('E站 ' + id);
  var coverMatch = html.match(/<img[^>]+(?:data-src|data-original|src)="([^"]+)"/i);
  var episodesMatch = html.match(/(?:更新到|更新至|更至)[\s\u00a0:：]*0*(\d+)/i);

  // 分集链接在 circuit_switch 区：<a class="circuit_switch1" href="/Index/video/<vid>.html">
  var chapters = [];
  var seen = {};
  var pattern = /<a[^>]+href="\/Index\/video\/(\d+)\.html"[^>]*>([\s\S]*?)<\/a>/gi;
  var match;
  while ((match = pattern.exec(html)) !== null) {
    var videoId = match[1];
    if (seen[videoId]) continue;
    seen[videoId] = true;
    var label = stripTags(match[2]) || ('第' + (chapters.length + 1) + '集');
    chapters.push({
      id: host.chapterId(id, videoId),
      source: host.source,
      title: label,
      currentEpisode: chapters.length + 1,
      videoId: videoId,
    });
  }

  var drama = {
    id: host.dramaId(id),
    source: host.source,
    sourceId: id,
    title: title,
    cover: coverMatch ? (coverMatch[1].indexOf('http') === 0 ? coverMatch[1] : SITE + coverMatch[1]) : '',
  };
  if (episodesMatch) drama.totalEpisode = Number(episodesMatch[1]);
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
  var videoId = String(chapter.videoId || '');
  if (!videoId) {
    var chapterId = String(chapter.id || '');
    var parts = chapterId.split(':');
    if (parts.length >= 3) videoId = parts[2];
  }
  if (!sourceId || !videoId) throw new Error('缺少播放参数');
  var playURL = SITE + '/Index/video/' + videoId + '.html';
  var html = fetchText(playURL);
  var iframeMatch = html.match(/src="(https?:\/\/player\.ezdmw\.com\/[^"]+)"/i);
  if (!iframeMatch) throw new Error('未找到播放器地址');
  var iframeURL = iframeMatch[1].replace(/&amp;/g, '&');
  // 播放器地址里的 title/quarterly 含空格与中文，请求前把空格转成 %20。
  var safeURL = iframeURL.replace(/ /g, '%20');
  var playerHTML = fetchText(safeURL, playURL);
  var videoMatch = playerHTML.match(/<video[^>]+src="([^"]+)"/i)
    || playerHTML.match(/<source[^>]+src="([^"]+)"/i);
  if (!videoMatch) throw new Error('该集暂时取不到播放地址');
  var media = videoMatch[1];
  if (media.indexOf('//') === 0) media = 'https:' + media;
  // 查询串里的中文与空格在 URL 中可以直接发；宿主会处理。
  return { url: media, referer: iframeURL };
};
