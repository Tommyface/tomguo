'use strict';
// 青空次元（sorani.net）：JSON API。
// 目录 GET /sorani-cms/api/video?page&size&categoryId
// 搜索 GET /sorani-cms/api/video/search?page&size&keyword
// 详情 GET /sorani-cms/api/video/<id>（含 episodes 数组）
// 播放 GET /sorani-cms/api/video/episode/<episodeId>/play
//
// 请求头必须带 Referer + Origin = https://www.sorani.net（否则 401）。
// 播放接口对未登录用户返回 canPlay=false + message「请先登录」，模块如实抛出。

var API = 'https://api.sorani.cc/sorani-cms/api';
var SITE = 'https://www.sorani.net';
var UA = 'Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';

var CATEGORIES = [
  { id: '1', name: 'TV番剧' },
  { id: '2', name: '剧场动画' },
  { id: '5', name: '特摄剧场' },
];

function apiGet(route, queryString) {
  var url = API + route + (queryString ? '?' + queryString : '');
  var response = host.fetch({
    url: url,
    method: 'GET',
    headers: {
      'User-Agent': UA,
      'Accept': 'application/json, text/plain, */*',
      'Accept-Language': 'zh-CN,zh;q=0.9',
      'Referer': SITE + '/',
      'Origin': SITE,
    },
    timeoutMs: 15000,
  });
  if (!response || response.status < 200 || response.status >= 400) {
    throw new Error('请求失败：' + (response ? response.status : 'no response'));
  }
  var text = response.body || (response.bodyBase64 ? String(host.base64Decode(response.bodyBase64)) : '');
  var json;
  try { json = JSON.parse(text); } catch (e) { throw new Error('接口数据格式无效'); }
  if (json && json.code && String(json.code) !== '200') {
    throw new Error('接口错误：' + (json.message || json.msg || ('码 ' + json.code)));
  }
  return json && json.data ? json.data : {};
}

function coverOf(row) {
  return row.cover || row.coverLarge || row.coverThumb || row.coverSmall || '';
}

function dramaFrom(row) {
  var id = String(row.id || '');
  return {
    id: host.dramaId(id),
    source: host.source,
    sourceId: id,
    title: String(row.title || ''),
    cover: coverOf(row),
    description: String(row.summary || ''),
    totalEpisode: Number(row.episodeCount || 0),
    statusText: String(row.statusText || ''),
  };
}

exports.categories = async function () {
  return CATEGORIES;
};

exports.catalogPage = async function (page, category) {
  var pageSize = 24;
  var qs = 'page=' + page + '&size=' + pageSize;
  var cat = String(category || '').trim();
  if (cat) qs += '&categoryId=' + encodeURIComponent(cat);
  var data = apiGet('/video', qs);
  var rows = data.records || [];
  var items = [];
  var seen = {};
  for (var i = 0; i < rows.length; i++) {
    var item = dramaFrom(rows[i]);
    if (!item.sourceId || seen[item.sourceId]) continue;
    seen[item.sourceId] = true;
    items.push(item);
  }
  var total = Number(data.total || 0);
  var hasMore = total > 0 ? page * pageSize < total : rows.length >= pageSize;
  return { items: items, hasMore: hasMore };
};

exports.search = async function (page, query) {
  var keyword = String(query || '').trim();
  if (!keyword) return { items: [], hasMore: false };
  var qs = 'page=' + page + '&size=24&keyword=' + encodeURIComponent(keyword);
  var data = apiGet('/video/search', qs);
  // 搜索接口返回的是裸数组（目录接口是 {records,total}），两种都兼容。
  var rows = Array.isArray(data) ? data : (data.records || []);
  var items = [];
  var seen = {};
  for (var i = 0; i < rows.length; i++) {
    var item = dramaFrom(rows[i]);
    if (!item.sourceId || seen[item.sourceId]) continue;
    seen[item.sourceId] = true;
    items.push(item);
  }
  var total = Number(data.total || 0);
  return { items: items, hasMore: total > 0 ? page * 24 < total : false };
};

exports.detail = async function (sourceId) {
  var id = String(sourceId || '').trim();
  if (!/^\d+$/.test(id)) throw new Error('剧集标识无效');
  var row = apiGet('/video/' + id, '');
  var drama = dramaFrom(row);
  if (drama.sourceId !== id) throw new Error('详情与请求剧集不符');
  var episodes = row.episodes || [];
  var chapters = [];
  for (var i = 0; i < episodes.length; i++) {
    var e = episodes[i];
    if (e.enabled === false) continue;
    var episodeId = String(e.episodeId || '');
    if (!episodeId || chapters.length >= 10000) continue;
    chapters.push({
      id: host.chapterId(id, episodeId),
      source: host.source,
      title: String(e.title || e.episodeLabel || ('第' + (i + 1) + '集')),
      currentEpisode: chapters.length + 1,
      episodeId: episodeId,
      videoUrl: String(e.playUrl || ''),
    });
  }
  return { drama: drama, chapters: chapters };
};

exports.resolveMedia = async function (task) {
  var chapter = (task && task.chapter) || {};
  var episodeId = String(chapter.episodeId || '');
  if (!episodeId) {
    var chapterId = String(chapter.id || '');
    var parts = chapterId.split(':');
    if (parts.length >= 3) episodeId = parts[2];
  }
  if (!episodeId) throw new Error('缺少播放参数');
  // 详情里已带 playUrl 时优先用，省一次请求。
  if (chapter.videoUrl) return { url: String(chapter.videoUrl), referer: SITE + '/' };
  var data = apiGet('/video/episode/' + encodeURIComponent(episodeId) + '/play', '');
  if (data.canPlay === false || !data.playUrl) {
    throw new Error(String(data.message || '该集需要登录或暂无播放地址'));
  }
  return { url: String(data.playUrl), referer: SITE + '/' };
};
