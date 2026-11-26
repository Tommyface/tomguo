'use strict';
// 橙果短剧（chengguodj.com）：Nuxt SSR，所有数据都在 <script type="application/json" data-nuxt-data> 里。
// 数据是「索引图」：对象字段值是数字索引，需按索引取真实值。
// 目录 /、详情 /drama/<slug>、播放 /play/<slug>/<集号>，M3U8 直链在播放页数据图里。

var SITE = 'https://chengguodj.com';
var rootGraph = null;
var UA = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3282.39 Safari/537.36';

function fetchText(url) {
  var response = host.fetch({
    url: url, method: 'GET',
    headers: { 'User-Agent': UA, Referer: SITE + '/' },
    timeoutMs: 30000,
  });
  if (!response || response.status < 200 || response.status >= 400) {
    throw new Error('请求失败：' + (response ? response.status : 'no response'));
  }
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}

// 取页面的 Nuxt 数据图并解索引引用。
function loadGraph(html) {
  var match = html.match(/<script type="application\/json" data-nuxt-data="nuxt-app"[^>]*>([\s\S]*?)<\/script>/i);
  if (!match) throw new Error('站点数据格式无效');
  try { return JSON.parse(match[1]); } catch (e) { throw new Error('站点数据解析失败'); }
}

function deref(graph, value, depth) {
  if (typeof value !== 'number' || value < 0 || value >= graph.length) return value;
  if (depth > 6) return value;
  var node = graph[value];
  if (node && typeof node === 'object' && !Array.isArray(node)) {
    var out = {};
    for (var key in node) out[key] = deref(graph, node[key], depth + 1);
    return out;
  }
  if (Array.isArray(node)) {
    var list = [];
    for (var i = 0; i < node.length && i < 500; i++) list.push(deref(graph, node[i], depth + 1));
    return list;
  }
  return node;
}

// 数据图里凡是带 slug/title/cover 的对象都是剧集节点。
function dramaNodes(graph) {
  var found = [];
  var seen = {};
  for (var i = 0; i < graph.length && i < 1200; i++) {
    var node = graph[i];
    if (!node || typeof node !== 'object' || Array.isArray(node)) continue;
    if (typeof node.slug !== 'number' || typeof node.title !== 'number') continue;
    var record = deref(graph, i, 0);
    if (!record || !record.slug || seen[record.slug]) continue;
    seen[record.slug] = true;
    found.push(record);
  }
  return found;
}

function toItem(record) {
  var cover = '';
  if (record.cover && typeof record.cover === 'object') {
    var u = record.cover.url;
    if (typeof u === 'number' && rootGraph) u = rootGraph[u];
    cover = String(u || record.cover.fallback_url || '');
    if (cover.indexOf('/_img/') === 0) cover = SITE + cover;
  }
  return {
    id: host.dramaId(record.slug),
    source: host.source,
    sourceId: String(record.slug || ''),
    title: String(record.title || ''),
    cover: cover,
    totalEpisode: Number(record.total_episode_count || record.published_episode_count || 0),
  };
}

exports.categories = async function () {
  return [{ id: '', name: '全部' }];
};

exports.catalogPage = async function (page, category) {
  var graph = loadGraph(fetchText(SITE + '/'));
  var all = dramaNodes(graph);
  var items = [];
  rootGraph = graph;
  for (var i = 0; i < all.length; i++) {
    if (!all[i].title) continue;
    items.push(toItem(all[i]));
  }
  // Nuxt 首页只提供一批编排数据，没有分页接口。
  if (page > 1) return { items: [], hasMore: false };
  return { items: items, hasMore: false };
};

exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword || page > 1) return { items: [], hasMore: false };
  var all = dramaNodes(loadGraph(fetchText(SITE + '/')));
  var items = [];
  for (var i = 0; i < all.length; i++) {
    if (String(all[i].title || '').indexOf(keyword) >= 0) items.push(toItem(all[i]));
  }
  return { items: items, hasMore: false };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^dj-[a-z0-9]+$/i.test(id)) throw new Error('剧集标识无效');
  var graph = loadGraph(fetchText(SITE + '/drama/' + id));
  var record = null;
  for (var i = 0; i < graph.length && i < 500; i++) {
    var node = graph[i];
    if (!node || typeof node !== 'object' || Array.isArray(node)) continue;
    if (typeof node.slug !== 'number' || graph[node.slug] !== id) continue;
    record = deref(graph, i, 0);
    break;
  }
  if (!record) throw new Error('未找到该剧详情');
  var drama = toItem(record);
  if (record.intro) drama.description = String(record.intro);
  var episodes = record.episodes || [];
  var chapters = [];
  for (var j = 0; j < episodes.length; j++) {
    var ep = episodes[j];
    if (!ep || !ep.number) continue;
    chapters.push({
      id: host.chapterId(id, String(ep.id || ep.number)),
      source: host.source,
      title: String(ep.title || ('第' + ep.number + '集')),
      currentEpisode: Number(ep.number),
      number: Number(ep.number),
      playRoute: String(ep.play_route || ''),
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
    var dramaId = String(drama.id || task.dramaId || '');
    sourceId = dramaId.indexOf(':') >= 0 ? dramaId.split(':')[1] : dramaId;
  }
  var number = Number(chapter.number || chapter.currentEpisode || '');
  if (!number) {
    var route = String(chapter.playRoute || chapter.pageUrl || '');
    var m = route.match(/\/(\d+)\/?$/);
    if (m) number = Number(m[1]);
  }
  if (!number) {
    var cid = String(chapter.id || '');
    var parts = cid.split(':');
    if (parts.length >= 4) number = Number(parts[3]);
  }
  if (!sourceId || !number) throw new Error('缺少播放参数');
  var playUrl = SITE + '/play/' + sourceId + '/' + number;
  var graph = loadGraph(fetchText(playUrl));
  for (var i = 0; i < graph.length; i++) {
    var value = graph[i];
    if (typeof value === 'string' && /https?:\/\/[^\s"']+\.m3u8[^\s"']*/.test(value)) {
      return { url: value, referer: SITE + '/' };
    }
  }
  throw new Error('该集暂时取不到播放地址');
};
