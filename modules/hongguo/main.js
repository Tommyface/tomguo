'use strict';
// 红果短剧网页版：页面内 _ROUTER_DATA 是 loaderData 数据图。
// 分类 /category/<route> → loaderData["category_$"].recommendList（24 条/页）
// 详情 /detail?series_id=<id> → loaderData.detail_page.seriesDetail.vid_list
// 播放 /player/<series>/<vid> → protectedPlayback=true 时无公开地址，
//   备用 API djapi.999888456.xyz 需要 AES 解密响应；不可达时如实报错。

var SITE = 'https://hongguoduanju.com';
var UA = 'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';
var PLAYLIST_API = 'https://djapi.999888456.xyz/api/hongguo/play';

var CATEGORIES = [
  { id: 'real-drama', name: '真人剧' },
  { id: 'comic-drama', name: '漫剧' },
  { id: 'ai-drama', name: 'AI剧' },
  { id: 'comic', name: '动漫' },
];

function fetchText(url) {
  var response = host.fetch({ url: url, method: 'GET', headers: { 'User-Agent': UA, Referer: SITE + '/' }, timeoutMs: 20000 });
  if (!response || response.status < 200 || response.status >= 400) {
    throw new Error('请求失败：' + (response ? response.status : 'no response'));
  }
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}

// 站点把 JSON 里的 "/" 写成 \u002F，解析前统一替换。
function loadRouter(html) {
  var index = html.indexOf('_ROUTER_DATA = ');
  if (index < 0) throw new Error('站点数据格式无效');
  var raw = html.slice(index + '_ROUTER_DATA = '.length).replace(/\\u002F/gi, '/');
  // JSON 后面还跟着脚本语句，用括号计数截出第一个完整对象。
  var depth = 0, inString = false, escaped = false, end = -1;
  for (var i = 0; i < raw.length; i++) {
    var ch = raw.charAt(i);
    if (escaped) { escaped = false; continue; }
    if (ch === '\\') { escaped = true; continue; }
    if (ch === '"') { inString = !inString; continue; }
    if (inString) continue;
    if (ch === '{') depth++;
    else if (ch === '}') { depth--; if (depth === 0) { end = i + 1; break; } }
  }
  if (end < 0) throw new Error('站点数据截断');
  try { return JSON.parse(raw.slice(0, end)); } catch (e) { throw new Error('站点数据解析失败'); }
}

function routerPage(html, names) {
  var data = loadRouter(html);
  var loader = data.loaderData || {};
  for (var i = 0; i < names.length; i++) {
    if (loader[names[i]]) return loader[names[i]];
  }
  return {};
}

function toItem(row, category) {
  var id = String(row.series_id || '');
  return {
    id: host.dramaId(id), source: host.source, sourceId: id,
    title: String(row.series_name || ''),
    cover: String(row.series_cover || ''),
    description: String(row.series_intro || ''),
    totalEpisode: Number(row.episode_cnt || 0),
    category: category || '',
  };
}

exports.categories = async function () { return CATEGORIES; };

exports.catalogPage = async function (page, category) {
  var cat = String(category || '').trim() || CATEGORIES[0].id;
  var url = SITE + '/category/' + cat + (page > 1 ? '?page=' + page : '');
  var stream = routerPage(fetchText(url), ['category_$', 'category_page']);
  var rows = stream.recommendList || [];
  var items = [], seen = {};
  for (var i = 0; i < rows.length; i++) {
    var item = toItem(rows[i], cat);
    if (!item.sourceId || seen[item.sourceId]) continue;
    seen[item.sourceId] = true;
    items.push(item);
  }
  var pagination = stream.pagination || {};
  var totalPages = Number(pagination.totalPages || 0);
  return { items: items, hasMore: totalPages > 0 ? page < totalPages : items.length >= 24 };
};

exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword || page > 1) return { items: [], hasMore: false };
  var html;
  try { html = fetchText(SITE + '/search/' + encodeURIComponent(keyword)); }
  catch (e) { return { items: [], hasMore: false }; }
  if (html.indexOf('_ROUTER_DATA = ') < 0) return { items: [], hasMore: false };
  var loader = loadRouter(html).loaderData || {};
  var rows = [];
  for (var key in loader) {
    var node = loader[key];
    if (node && Array.isArray(node.recommendList)) rows = rows.concat(node.recommendList);
    if (node && Array.isArray(node.videoList)) rows = rows.concat(node.videoList);
  }
  var items = [], seen = {};
  for (var i = 0; i < rows.length; i++) {
    var item = toItem(rows[i], '');
    if (!item.sourceId || seen[item.sourceId]) continue;
    seen[item.sourceId] = true;
    items.push(item);
  }
  return { items: items, hasMore: false };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim().replace(/^hg-series-v1:/, '');
  if (!/^\d+$/.test(id)) throw new Error('剧集标识无效');
  var stream = routerPage(fetchText(SITE + '/detail?series_id=' + encodeURIComponent(id)), ['detail_page', 'detail_$']);
  var detail = stream.seriesDetail || {};
  var drama = toItem(detail, '');
  if (drama.sourceId !== id) throw new Error('详情与请求剧集不符');
  var vids = detail.vid_list || [];
  var chapters = [];
  for (var i = 0; i < vids.length; i++) {
    var vid = String(vids[i] || '');
    if (!/^\d+$/.test(vid)) continue;
    chapters.push({
      id: host.chapterId(id, vid), source: host.source,
      title: '第' + (i + 1) + '集', currentEpisode: i + 1,
      videoId: vid,
      pageUrl: SITE + '/player/' + id + '/' + vid,
    });
  }
  if (chapters.length) drama.totalEpisode = chapters.length;
  return { drama: drama, chapters: chapters };
};

exports.resolveMedia = async function (task) {
  var chapter = (task && task.chapter) || {};
  var drama = (task && task.drama) || {};
  var sourceId = String(drama.sourceId || '');
  if (!sourceId) {
    var id0 = String(drama.id || task.dramaId || '').replace(/^hg-series-v1:/, '');
    sourceId = id0.indexOf(':') >= 0 ? id0.split(':')[1] : id0;
  }
  var videoId = String(chapter.videoId || '');
  if (!videoId) { var parts = String(chapter.id || '').split(':'); if (parts.length >= 3) videoId = parts[2]; }
  if (!sourceId || !videoId) throw new Error('缺少播放参数');
  var pageUrl = SITE + '/player/' + sourceId + '/' + videoId;
  var page = routerPage(fetchText(pageUrl), ['player_(series_id)/(vid)/page', 'player_page', 'player_$']);
  var info = page.video_player_info || {};
  var addresses = [];
  for (var key in info) {
    var value = info[key];
    if (typeof value === 'string' && /^https?:\/\//.test(value) && /\.m3u8|\.mp4/.test(value)) addresses.push(value);
  }
  if (addresses.length) return { url: addresses[0], referer: SITE + '/' };
  // 备用播放接口需要 AES 解密响应，当前运行环境未注入解密能力；如实报告。
  var ref = JSON.stringify({ content_type: 1004, series_id: sourceId, video_id: videoId, video_platform: 3 });
  try {
    var body = fetchText(PLAYLIST_API + '?id=' + encodeURIComponent(host.base64Encode ? host.base64Encode(ref) : ref));
    if (body && body.indexOf('v2.') !== 0) {
      var parsed = JSON.parse(body);
      var urls = parsed.key_urls || [];
      if (urls.length && urls[0].url) return { url: urls[0].url, referer: 'https://novel.snssdk.com/' };
    }
  } catch (e) { /* 备用接口不可达时走下面的明确报错 */ }
  throw new Error('红果该集未提供公开播放地址，可能需要登录或 App 授权');
};
