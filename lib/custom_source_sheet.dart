import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'custom_source.dart';

/// 添加/编辑自定义站源的底部表单。
///
/// 分「基础」与「高级」两段：基础只需名称 + 地址 + 类型；高级填路由正则
/// （苹果CMS）或 AES 参数。默认提供常见苹果CMS 模板的一键填充，降低门槛。
class CustomSourceSheet extends StatefulWidget {
  const CustomSourceSheet({super.key, this.initial});

  final CustomSource? initial;

  /// 以模态底部页方式打开，返回保存后的配置（null 表示取消）。
  static Future<CustomSource?> show(
    BuildContext context, {
    CustomSource? initial,
  }) {
    return showModalBottomSheet<CustomSource>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (_) => CustomSourceSheet(initial: initial),
    );
  }

  @override
  State<CustomSourceSheet> createState() => _CustomSourceSheetState();
}

class _CustomSourceSheetState extends State<CustomSourceSheet> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _name;
  late final TextEditingController _baseUrl;
  late final TextEditingController _description;
  late String _type;

  // 苹果CMS
  late final TextEditingController _detailPattern;
  late final TextEditingController _playPattern;
  late final TextEditingController _detailTpl;
  late final TextEditingController _listTpl;
  late final TextEditingController _searchTpl;

  // AES
  late final TextEditingController _apiUrl;
  late final TextEditingController _aesKey;
  late final TextEditingController _aesIv;
  late final TextEditingController _signKey;
  late final TextEditingController _signTpl;
  late final TextEditingController _catalogPath;
  late final TextEditingController _detailPath;
  late final TextEditingController _playPath;
  bool _keyIsHex = false;
  bool _advanced = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    final s = widget.initial;
    _name = TextEditingController(text: s?.name ?? '');
    _baseUrl = TextEditingController(text: s?.baseUrl ?? '');
    _description = TextEditingController(text: s?.description ?? '');
    _type = s?.type ?? 'maccms';

    _detailPattern = TextEditingController(
      text: s?.detailPathPattern ?? r'^/voddetail/([0-9]+)\.html$',
    );
    _playPattern = TextEditingController(
      text: s?.playPathPattern ?? r'^/vodplay/[0-9]+-[0-9]+-[0-9]+\.html$',
    );
    _detailTpl = TextEditingController(
      text: s?.detailUrlTemplate ?? '{base}/voddetail/{id}.html',
    );
    _listTpl = TextEditingController(
      text: s?.listUrlTemplate ?? '{base}/vodtype/1-{page}.html',
    );
    _searchTpl = TextEditingController(
      text: s?.searchUrlTemplate ?? '{base}/search?wd={query}&page={page}',
    );

    _apiUrl = TextEditingController(text: s?.apiUrl ?? '');
    _aesKey = TextEditingController(text: s?.aesKey ?? '');
    _aesIv = TextEditingController(text: s?.aesIv ?? '');
    _signKey = TextEditingController(text: s?.signKey ?? '');
    _signTpl = TextEditingController(text: s?.signTemplate ?? '');
    _catalogPath = TextEditingController(text: s?.catalogPath ?? '/api/v1/list');
    _detailPath = TextEditingController(text: s?.detailApiPath ?? '/api/v1/detail');
    _playPath = TextEditingController(text: s?.playApiPath ?? '/api/v1/play');
    _keyIsHex = s?.keyIsHex ?? false;
  }

  @override
  void dispose() {
    for (final c in [
      _name, _baseUrl, _description, _detailPattern, _playPattern,
      _detailTpl, _listTpl, _searchTpl, _apiUrl, _aesKey, _aesIv,
      _signKey, _signTpl, _catalogPath, _detailPath, _playPath,
    ]) {
      c.dispose();
    }
    super.dispose();
  }

  void _applyTemplate(int index) {
    // 常见苹果CMS 路由模板，减少手工填写。
    const templates = [
      {
        'name': '常规 (voddetail/vodplay)',
        'detail': r'^/voddetail/([0-9]+)\.html$',
        'play': r'^/vodplay/[0-9]+-[0-9]+-[0-9]+\.html$',
        'detailUrl': '{base}/voddetail/{id}.html',
        'list': '{base}/vodtype/1-{page}.html',
        'search': '{base}/search?wd={query}&page={page}',
      },
      {
        'name': '花生 (zywview/zywplay)',
        'detail': r'^/zywview/([0-9]+)\.html$',
        'play': r'^/zywplay/[0-9]+-[0-9]+-[0-9]+\.html$',
        'detailUrl': '{base}/zywview/{id}.html',
        'list': '{base}/zywtype/27.html',
        'search': '{base}/search.html?searchtype=1&searchword={query}',
      },
      {
        'name': 'vod (play/id/sid/nid)',
        'detail': r'^/vod/detail/id/([0-9]+)\.html$',
        'play': r'^/vod/play/id/[0-9]+/sid/[0-9]+/nid/[0-9]+\.html$',
        'detailUrl': '{base}/vod/detail/id/{id}.html',
        'list': '{base}/vod/type/id/1/page/{page}.html',
        'search': '{base}/vod/search/wd/{query}/page/{page}.html',
      },
    ];
    final t = templates[index];
    setState(() {
      _detailPattern.text = t['detail']!;
      _playPattern.text = t['play']!;
      _detailTpl.text = t['detailUrl']!;
      _listTpl.text = t['list']!;
      _searchTpl.text = t['search']!;
    });
  }

  void _save() {
    if (!_formKey.currentState!.validate()) return;
    final cfg = CustomSource(
      id: widget.initial?.id ?? '',
      name: _name.text.trim(),
      description: _description.text.trim(),
      baseUrl: _baseUrl.text.trim(),
      type: _type,
      detailPathPattern: _detailPattern.text.trim(),
      playPathPattern: _playPattern.text.trim(),
      detailUrlTemplate: _detailTpl.text.trim(),
      listUrlTemplate: _listTpl.text.trim(),
      searchUrlTemplate: _searchTpl.text.trim(),
      apiUrl: _apiUrl.text.trim(),
      aesKey: _aesKey.text.trim(),
      aesIv: _aesIv.text.trim(),
      keyIsHex: _keyIsHex,
      signKey: _signKey.text.trim(),
      signTemplate: _signTpl.text.trim(),
      catalogPath: _catalogPath.text.trim(),
      detailApiPath: _detailPath.text.trim(),
      playApiPath: _playPath.text.trim(),
    );
    final problem = cfg.validate();
    if (problem != null) {
      setState(() => _error = problem);
      return;
    }
    Navigator.of(context).pop(cfg);
  }

  @override
  Widget build(BuildContext context) {
    final bottom = MediaQuery.viewInsetsOf(context).bottom;
    return Padding(
      padding: EdgeInsets.only(bottom: bottom),
      child: SingleChildScrollView(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
          child: Form(
            key: _formKey,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  widget.initial == null ? '添加自定义站源' : '编辑自定义站源',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: 4),
                Text(
                  '配置保存在本机，无需重新构建 App。',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
                const SizedBox(height: 16),
                _field(_name, '名称', hint: '如：我的短剧站'),
                const SizedBox(height: 12),
                _field(
                  _baseUrl,
                  '站点地址',
                  hint: 'https://example.com',
                  keyboard: TextInputType.url,
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<String>(
                  initialValue: _type,
                  decoration: const InputDecoration(
                    labelText: '站源类型',
                    border: OutlineInputBorder(),
                  ),
                  items: const [
                    DropdownMenuItem(value: 'maccms', child: Text('苹果CMS（推荐，免构建）')),
                    DropdownMenuItem(value: 'aes', child: Text('AES 接口（需密钥，部分免构建）')),
                  ],
                  onChanged: (v) => setState(() => _type = v ?? 'maccms'),
                ),
                const SizedBox(height: 12),
                _field(_description, '备注（可选）'),
                const SizedBox(height: 16),
                if (_type == 'maccms') _maccmsSection() else _aesSection(),
                const SizedBox(height: 12),
                ExpansionTile(
                  initiallyExpanded: _advanced,
                  title: const Text('高级选项'),
                  children: _type == 'maccms'
                      ? [
                          _field(_playPattern, '播放页 path 正则'),
                          const SizedBox(height: 8),
                          _field(_searchTpl, '搜索 URL 模板'),
                        ]
                      : [
                          _field(_signTpl, '签名拼接模板'),
                          const SizedBox(height: 8),
                          SwitchListTile(
                            title: const Text('密钥为十六进制'),
                            value: _keyIsHex,
                            onChanged: (v) => setState(() => _keyIsHex = v),
                          ),
                        ],
                ),
                if (_error != null) ...[
                  const SizedBox(height: 12),
                  Text(_error!, style: const TextStyle(color: Colors.red)),
                ],
                const SizedBox(height: 20),
                FilledButton(
                  onPressed: _save,
                  child: const Text('保存并启用'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _maccmsSection() => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      Row(
        children: [
          const Text('路由模板：'),
          const SizedBox(width: 8),
          Expanded(
            child: Wrap(
              spacing: 8,
              children: [
                for (var i = 0; i < 3; i++)
                  OutlinedButton(
                    onPressed: () => _applyTemplate(i),
                    child: Text(['常规', '花生', 'vod'][i]),
                  ),
              ],
            ),
          ),
        ],
      ),
      const SizedBox(height: 10),
      _field(_detailPattern, '详情页 path 正则（必填）', hint: r'^/voddetail/([0-9]+)\.html$'),
      const SizedBox(height: 10),
      _field(_detailTpl, '详情页 URL 模板', hint: '{base}/voddetail/{id}.html'),
      const SizedBox(height: 10),
      _field(_listTpl, '目录 URL 模板', hint: '{base}/vodtype/1-{page}.html'),
    ],
  );

  Widget _aesSection() => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      _field(_apiUrl, 'API 地址（可选，默认同站点）'),
      const SizedBox(height: 10),
      _field(_aesKey, 'AES 密钥（必填）'),
      const SizedBox(height: 10),
      _field(_aesIv, 'AES IV（可选）'),
      const SizedBox(height: 10),
      _field(_signKey, '签名密钥（可选）'),
      const SizedBox(height: 10),
      _field(_catalogPath, '目录接口 path'),
      const SizedBox(height: 10),
      _field(_detailPath, '详情接口 path'),
      const SizedBox(height: 10),
      _field(_playPath, '播放接口 path'),
      const SizedBox(height: 12),
      const Text(
        '提示：仅支持「AES-CBC + 固定签名」同构接口；动态密钥协商或私有协议仍需专属适配。',
        style: TextStyle(fontSize: 12, color: Colors.orange),
      ),
    ],
  );

  Widget _field(
    TextEditingController controller,
    String label, {
    String? hint,
    TextInputType? keyboard,
  }) => TextFormField(
    controller: controller,
    keyboardType: keyboard,
    decoration: InputDecoration(
      labelText: label,
      hintText: hint,
      border: const OutlineInputBorder(),
    ),
    validator: (v) {
      if (label.contains('必填') && (v == null || v.trim().isEmpty)) {
        return '此项必填';
      }
      return null;
    },
  );
}
