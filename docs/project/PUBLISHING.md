# 发布在线电子书

本次交付为完整体系v0.7，接口入口与指认修订于2026-10-11，项目快照project-v0.7.8。完整v0.5、结构核心v0.2冻结归档及v0.6末次工作稿快照保留。网站与完整阅读稿都由docs生成。

## 将v0.7上传到原仓库

仓库：[liamamilin/cognitive-language-engineering](https://github.com/liamamilin/cognitive-language-engineering)。

1. 解压 `CLE_v0.7_GitHub上传包.zip`，打开其中的 `upload` 文件夹。
2. 在原仓库的main分支选择 **Add file → Upload files**，将 `upload` 内的文件和文件夹上传到仓库根目录，覆盖同路径文件。不要上传ZIP本身，也不要把外层 `upload` 作为目录上传。
3. 确认上传后的根目录仍直接包含 `docs`、`scripts`、`overrides`、`mkdocs.yml`、`README.md` 和 `.github/workflows/book.yml`。提交说明可写 `Release CLE v0.7`。
4. 提交后打开 **Actions**，等待 `Build and publish CLE book` 的构建与部署成功。
5. 打开[在线阅读入口](https://liamamilin.github.io/cognitive-language-engineering/)，刷新页面并确认首页显示 **完整体系 v0.7** 和作者署名，并确认日期为2026-10-11，并确认M1在特征组之前有所指（Referent）组及R-C2指认；接口总数为82；七项原则页应有P1—P7的逐项讲解，模型卡片应显示语言模式链接、“核心句”和“展开”；R-C2核心句应为“先描述你注意到的情况，再找合适的称呼”，R-C1核心句应为“先比较实例，试写含义，再用新例检验”，I-C3核心句应为“先比较各自的例子，再约定共同用法”，A-P1展开应直接提出行动要求，完成检查另说明到达与成立依据，再确认首页项目快照为project-v0.7.8，并查看术语与阅读路线。

上传包合并此前尚未同步的正文与阅读体验修改，以及本次v0.7更新；适用于原仓库。包内另附完整项目ZIP，供保存完整源码或恢复项目，不需要把它作为压缩文件上传到仓库。

提交、构建和部署分别确认。本次交付没有执行远程提交，也没有声称v0.7已经上线。上传的文件若缺失或同路径未覆盖，先对照上传清单补齐。

## 构建与后续维护

Markdown → MkDocs Material → 静态阅读页面 → GitHub Pages。main提交触发 `.github/workflows/book.yml`；PR只做构建检查，手动运行也可触发部署。

正文修改后同步索引、版本说明与派生检查材料，执行README中的检查和严格构建。工作流检查登记表、学习连接、正文链接、结构核心与生成页面；检查失败时不会继续部署。

仓库Settings → Pages → Build and deployment → Source使用GitHub Actions。部署失败时查看对应任务日志，按实际原因修正或重跑；上线以成功部署和实际页面为准。

结构核心使用独立阅读模板，同一次构建输出至 `essentials/index.html`。完整体系首页“阅读路线”保留入口，核心版保留返回和追溯链接。

单文件Markdown与HTML阅读稿通过 `python scripts/export_book.py --output-dir reading` 重新生成，不单独改正文。网站展示成功部署的提交；旧版本由归档和Git历史追溯。

## 历史记录

已知2026-10-01用户上传v0.6后，[该次提交](https://github.com/liamamilin/cognitive-language-engineering/commit/e604dcefd3f463b807d98c7e53a1134cf5e9f801)的部署成功。该记录只说明当时内容上线；此后的本地修改和v0.7须以新的提交与部署确认。

首次导入所用的source-import.zip和兼容工作流属于历史安排；原仓库已有正文，本次按现有路径更新。

## 官方参考

- [MkDocs：Deploying your docs](https://www.mkdocs.org/user-guide/deploying-your-docs/)
- [Material for MkDocs：Publishing your site](https://squidfunk.github.io/mkdocs-material/publishing-your-site/)
- [GitHub：Deploying your website automatically](https://docs.github.com/en/get-started/start-your-journey/deploying-your-website-automatically)
