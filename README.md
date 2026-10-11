# 认知语言工程｜完整体系 v0.7

2026-10-11 · 发布稿，打磨指认并审查结果前置；项目快照project-v0.7.8。承接v0.6的概念审查、隐藏知识审查和查漏补缺，完成阅读润色。学习主线为 **Principle → Model → 接口 → 算子 → 语言模式**；掌握后的应用步骤另列。

从[开始阅读](docs/index.md)和[学习主线](docs/learning/MAINLINE.md)进入。正文权威来源为docs，网站和完整阅读稿由同一来源生成。作者：lin.mi · lin.mi.official@gmail.com。

本版保留原81接口，新增复合接口R-C2指认，共82接口；37算子、39语言模式、13协议及支撑内容继续复用。M1按所指、特征、概念、表达组织阅读，指认补明所指再进入特征选择；原接口定义与冻结内容保留，不新增效果证据。[修订范围](docs/project/REVIEW_REPORT.md)与[变更记录](docs/project/CHANGELOG.md)可追溯。

## 本地检查与构建

```bash
python -m pip install -r requirements.txt
python scripts/update_registry.py --check
python scripts/check_mainline.py
python scripts/check_docs.py
python scripts/check_essentials.py
python -m mkdocs build --strict
python scripts/check_site.py
python scripts/export_book.py --output-dir reading
```

## 上传与版本

同步目标为原仓库liamamilin/cognitive-language-engineering的main分支；[网页上传步骤](docs/project/PUBLISHING.md)说明怎样更新。main提交触发电子书构建与部署，上线须确认Actions和首页版本；本次交付没有代为提交或部署。

完整v0.5和结构核心v0.2冻结原文保留；v0.6末次工作稿快照存于archive/CLE_v0.6_末次工作稿快照.zip。v0.7是本次交付版本，后续修订继续记录。组件和整套教学效果默认E0，文档检查不证明效果。
