# Changelog

## [0.1.2](https://github.com/rshade/finfocus-plugin-aws-ce/compare/v0.1.1...v0.1.2) (2026-10-22)


### Features

* **carbon:** add opt-in historical AWS emissions reports ([#87](https://github.com/rshade/finfocus-plugin-aws-ce/issues/87)) ([1e27059](https://github.com/rshade/finfocus-plugin-aws-ce/commit/1e27059db5f7fbe0fdedfc85d45824ce2eb37814)), closes [#29](https://github.com/rshade/finfocus-plugin-aws-ce/issues/29)


### Bug Fixes

* **deps:** update go dependencies ([#84](https://github.com/rshade/finfocus-plugin-aws-ce/issues/84)) ([071a3a6](https://github.com/rshade/finfocus-plugin-aws-ce/commit/071a3a6c0f1c82fe3d2da4355b090455a8750551))


### Documentation

* link AWS Cost Explorer and FinFocus from the README ([#85](https://github.com/rshade/finfocus-plugin-aws-ce/issues/85)) ([1178fa6](https://github.com/rshade/finfocus-plugin-aws-ce/commit/1178fa6147b77f7d847361529196b68e6f5d6ba5))

## [0.1.1](https://github.com/rshade/finfocus-plugin-aws-ce/compare/v0.1.0...v0.1.1) (2026-10-21)


### Features

* **anomalies:** proxy Cost Explorer anomalies ([#38](https://github.com/rshade/finfocus-plugin-aws-ce/issues/38)) ([e5c0545](https://github.com/rshade/finfocus-plugin-aws-ce/commit/e5c054563aef1756bec3d093e661331d4b830050))
* **budgets:** read AWS Budgets ([#8](https://github.com/rshade/finfocus-plugin-aws-ce/issues/8)) ([646577a](https://github.com/rshade/finfocus-plugin-aws-ce/commit/646577a3b3ad76e377e4685056ce3b22cc5df490))
* bump finfocus-spec to v0.7.8 and advertise plugin settings ([d739dd8](https://github.com/rshade/finfocus-plugin-aws-ce/commit/d739dd850d0fc743f3c34a3f171dc938b2c76248))
* **client:** retrieve AWS optimization recommendations ([5d3c9cc](https://github.com/rshade/finfocus-plugin-aws-ce/commit/5d3c9ccb63c5b8149b82cd169e8bbe5b60dbbce3))
* complete actual-cost foundation and official conformance ([8b11e44](https://github.com/rshade/finfocus-plugin-aws-ce/commit/8b11e44bb88eeaf0e75db9812abca09c5e81ba7a))
* **cors:** set allowed origins ([#54](https://github.com/rshade/finfocus-plugin-aws-ce/issues/54)) ([f644d09](https://github.com/rshade/finfocus-plugin-aws-ce/commit/f644d090aa8f1124afc82b5aef845d85f13a23ee))
* **health:** enable SDK health endpoint ([#43](https://github.com/rshade/finfocus-plugin-aws-ce/issues/43)) ([63f31a8](https://github.com/rshade/finfocus-plugin-aws-ce/commit/63f31a85609474006a46d2b81bd927d75a8beed8))
* **plugin:** advertise and verify recommendations over RPC ([62cea1f](https://github.com/rshade/finfocus-plugin-aws-ce/commit/62cea1fd3a9fd6c5c7842291879c503cd0581e4d))
* **pricing:** accept any AWS resource ARN ([6198fa0](https://github.com/rshade/finfocus-plugin-aws-ce/commit/6198fa091cf76cff7efc2cd8f241647f7dd83639))
* **pricing:** add non-EC2 reservation purchase recommendations ([739fe62](https://github.com/rshade/finfocus-plugin-aws-ce/commit/739fe624462527512dccc7eeb056ac78a59b0cee)), closes [#33](https://github.com/rshade/finfocus-plugin-aws-ce/issues/33)
* **pricing:** make the Spot comparison opt in ([ba9d3c6](https://github.com/rshade/finfocus-plugin-aws-ce/commit/ba9d3c6a90d129622ddfd1c539ca0ec929815b32))
* **pricing:** map AWS optimization recommendations ([d8bbc6f](https://github.com/rshade/finfocus-plugin-aws-ce/commit/d8bbc6fdc5578214be90926ff046ac0e3412ee26))
* **pricing:** proxy Cost Explorer forecasts ([#36](https://github.com/rshade/finfocus-plugin-aws-ce/issues/36)) ([0830356](https://github.com/rshade/finfocus-plugin-aws-ce/commit/08303569188ee7e218e28a64b8d9e6378b6665ad))
* **pricing:** queue, share and batch Cost Explorer requests ([9383ce4](https://github.com/rshade/finfocus-plugin-aws-ce/commit/9383ce42fa23137cda4a2e50386c10bd64b1fd53))
* **pricing:** quote EC2 Spot price against on-demand in GetProjectedCost ([f605a99](https://github.com/rshade/finfocus-plugin-aws-ce/commit/f605a99623cc0b8e480ce98e3d23d65a8b7184d7)), closes [#56](https://github.com/rshade/finfocus-plugin-aws-ce/issues/56) [#34](https://github.com/rshade/finfocus-plugin-aws-ce/issues/34)
* **pricing:** serve filtered AWS recommendations ([26a449e](https://github.com/rshade/finfocus-plugin-aws-ce/commit/26a449eef17d9da72d5bf6d54dfcb4e628de8970))
* **web:** serve Connect from configuration ([#49](https://github.com/rshade/finfocus-plugin-aws-ce/issues/49)) ([2260626](https://github.com/rshade/finfocus-plugin-aws-ce/commit/2260626d45ffa9c29cad04fa5fbb61e457547da8))


### Bug Fixes

* answer Supports by resource type when no id is sent ([8298a53](https://github.com/rshade/finfocus-plugin-aws-ce/commit/8298a538efc21cae8b8e1b9dba2d5a51750e688f))
* **ci:** exclude generated Claude prose from reviewdog ([33d5eb9](https://github.com/rshade/finfocus-plugin-aws-ce/commit/33d5eb966b47efe80b0899d75f1b42a75a03bdb5))
* **ci:** publish Vale results as a check run ([7353410](https://github.com/rshade/finfocus-plugin-aws-ce/commit/73534109003f3582033c09b9faeea81eb6a3039d))
* **ci:** report Vale findings as annotations ([d7f3739](https://github.com/rshade/finfocus-plugin-aws-ce/commit/d7f3739fa9fadb9fdef0de2d743708e58c790cc1))
* **client:** send valid resource-level filters and treat empty days as no data ([1a95478](https://github.com/rshade/finfocus-plugin-aws-ce/commit/1a954784dfbfa700442af02e5b1321a9caf29b21))
* **deps:** update go dependencies ([7a25d4c](https://github.com/rshade/finfocus-plugin-aws-ce/commit/7a25d4c19044f1e93ff06b816e86cfcb8a682dc7))
* **pricing:** isolate per-request pacing and bound the discovery cache ([68024c5](https://github.com/rshade/finfocus-plugin-aws-ce/commit/68024c59109a741d26f54ce202ddb9ca7eb4965d))
* **pricing:** name the denied AWS operation and document account prerequisites ([bc7d3f5](https://github.com/rshade/finfocus-plugin-aws-ce/commit/bc7d3f5c5230d65e2efe6d8d0768456ec2b92d8a))
* **pricing:** project the UTC month for accounts and services only ([054462c](https://github.com/rshade/finfocus-plugin-aws-ce/commit/054462c4fbaed7a8642be27f617588ee84ed7165))
* **pricing:** treat a blank savings plan page as empty ([43e49c2](https://github.com/rshade/finfocus-plugin-aws-ce/commit/43e49c2add1c1ec597c1ff1957533f599ce822f8))
* **pricing:** treat an empty Savings Plans response as no recommendations ([11a44b7](https://github.com/rshade/finfocus-plugin-aws-ce/commit/11a44b7e889bb6c7bd8ce6e5980c2bb42be078cd))
* **recommendations:** honor scoped sources and AWS target semantics ([cd3d4cf](https://github.com/rshade/finfocus-plugin-aws-ce/commit/cd3d4cf63ecf77e1ea54bba39b45e10f9b56d8ca))
* **recommendations:** keep payer Savings Plans details without account ([bdd2cc9](https://github.com/rshade/finfocus-plugin-aws-ce/commit/bdd2cc9f69274c73f69a893f7aa65fd33f25d048))


### Documentation

* add API configuration and deployment guides ([aadf9a2](https://github.com/rshade/finfocus-plugin-aws-ce/commit/aadf9a220efa29f71ad6187ef3b216ccb4507353))
* add API, deployment, and configuration guides ([#44](https://github.com/rshade/finfocus-plugin-aws-ce/issues/44)) ([45e7f9a](https://github.com/rshade/finfocus-plugin-aws-ce/commit/45e7f9af941ce6950c78876840e62b6aebf06ce5))
* clear Vale findings and require checks during edits ([0ddbfe7](https://github.com/rshade/finfocus-plugin-aws-ce/commit/0ddbfe7fc2e76d69f4fbfce02df08de45d6abb86))
* clear Vale findings in changed guides ([837c96b](https://github.com/rshade/finfocus-plugin-aws-ce/commit/837c96b163d151f208daba4b1612658ca6f99f74))
* document and verify AWS recommendations ([b487874](https://github.com/rshade/finfocus-plugin-aws-ce/commit/b48787454a95d63df549460fe9a072b465bf0492))
* plan AWS recommendation implementation ([0eb8111](https://github.com/rshade/finfocus-plugin-aws-ce/commit/0eb81116ba43bef91e394e2df1b8e40155c22761))
* prepare documentation and spec v0.7.9 for production release ([5e69515](https://github.com/rshade/finfocus-plugin-aws-ce/commit/5e6951537b11e5d7890aa24474946da5ce596264))
* **recommendations:** clarify contracts and clear Vale findings ([16cde6e](https://github.com/rshade/finfocus-plugin-aws-ce/commit/16cde6e1fdfe17520b715d030b48c3ed44b58149))
* specify AWS optimization recommendations for issue 13 ([f199dfb](https://github.com/rshade/finfocus-plugin-aws-ce/commit/f199dfb9c49f039863c87b3ebfc1c1d12a508655))
* update roadmap after v0.1.0 release ([180e484](https://github.com/rshade/finfocus-plugin-aws-ce/commit/180e48411ba70c2628e76230ad8881239570caa2))

## 0.1.0 (2026-10-07)


### Features

* accept per-request AWS credentials (CE-1.6) ([009f7e8](https://github.com/rshade/finfocus-plugin-aws-ce/commit/009f7e8b88137e089ea9c2f0ffcb59dd82e18564))
* attach proto ErrorCode details to RPC errors (CE-1.5) ([8a87a7b](https://github.com/rshade/finfocus-plugin-aws-ce/commit/8a87a7b496bf3f450e7c2e4754504ca42c068fb8))
* **ci:** establish CI/CD infrastructure foundation with E2E testing ([#35](https://github.com/rshade/finfocus-plugin-aws-ce/issues/35)) ([bb6c2e7](https://github.com/rshade/finfocus-plugin-aws-ce/commit/bb6c2e766ef1e574152b058af2e49913a3de6591)), closes [#7](https://github.com/rshade/finfocus-plugin-aws-ce/issues/7)
* expose Cost Explorer FOCUS metadata (CE-6.5) ([d71d58e](https://github.com/rshade/finfocus-plugin-aws-ce/commit/d71d58e61733663dd4c06485218cc84055bd9256))
* implement Supports for aws id or ARN (CE-1.2) ([727679a](https://github.com/rshade/finfocus-plugin-aws-ce/commit/727679a7dda55802d88a47b2092aeb990c958a3d))
* log trace id from request context (CE-1.4) ([4dd8a34](https://github.com/rshade/finfocus-plugin-aws-ce/commit/4dd8a34dbe92bff1b7cbb01dc1ef6b42970eec65))
* map reservation and savings plan rows into FOCUS (CE-6.4) ([68e327d](https://github.com/rshade/finfocus-plugin-aws-ce/commit/68e327d771db1ee234d26ccdf26f6d9aeaebec09))
* **plugin:** implement AWS Cost Explorer plugin with full test coverage ([#5](https://github.com/rshade/finfocus-plugin-aws-ce/issues/5)) ([a8d74eb](https://github.com/rshade/finfocus-plugin-aws-ce/commit/a8d74eb8c158de0bfdaba63a93362bd32adad8f8)), closes [#2](https://github.com/rshade/finfocus-plugin-aws-ce/issues/2)
* **pricing:** add ARN support to GetActualCost for precise resource identification ([#16](https://github.com/rshade/finfocus-plugin-aws-ce/issues/16)) ([005723b](https://github.com/rshade/finfocus-plugin-aws-ce/commit/005723beead51f0dbe5aeda286c972656f5ce121)), closes [#14](https://github.com/rshade/finfocus-plugin-aws-ce/issues/14)
* report aws-ce plugin info for actual costs (CE-1.3) ([857e019](https://github.com/rshade/finfocus-plugin-aws-ce/commit/857e019675f75e3606185111907a01123566f8b7))


### Bug Fixes

* budget custom CE retries (CE-6.9, CE-R.5, CE-R.6) ([bab371b](https://github.com/rshade/finfocus-plugin-aws-ce/commit/bab371bf32c53b889ac7b36b739be124b12da53d))
* compare contract cost totals as exact decimals (CE-6.1) ([6a716dc](https://github.com/rshade/finfocus-plugin-aws-ce/commit/6a716dc92e10287cde33cc01664391a626238d59))
* correct cost attribution and cache isolation ([305f9a9](https://github.com/rshade/finfocus-plugin-aws-ce/commit/305f9a9f1f34492aacae581e4e89ef915601ef96))
* correct cost attribution and cache isolation ([f226d87](https://github.com/rshade/finfocus-plugin-aws-ce/commit/f226d87448819322381bb6e5407187f9e118fc9f))
* correct Supports capabilities (CE-6.10, CE-R.2) ([2608c7a](https://github.com/rshade/finfocus-plugin-aws-ce/commit/2608c7ae1f046ff783566e06415c11ab39c015cd))
* enforce actual-cost-only behavior and remove test stubs (CE-6.10) ([669ebb4](https://github.com/rshade/finfocus-plugin-aws-ce/commit/669ebb4618b6302897f70603bdbcd95ade04e860))
* hash cost cache files and meter Cost Explorer pages (CE-6.8) ([3e721c0](https://github.com/rshade/finfocus-plugin-aws-ce/commit/3e721c0fcbc3ce3ff7bd15855d077512b3b1d6ab))
* honor cost explorer contract for sums, dates, and errors (CE-6.1) ([c42b969](https://github.com/rshade/finfocus-plugin-aws-ce/commit/c42b9698b82d584021973ce207474e831c7c8025))
* isolate account caches (REL-4, CE-6.8, CE-1.1, CE-R.3) ([0966aaf](https://github.com/rshade/finfocus-plugin-aws-ce/commit/0966aaf8eb2cfd7be0c15345812cf945e2117dfb))
* keep commitment ids on zero-usage and non-service groups (CE-6.4) ([aee466e](https://github.com/rshade/finfocus-plugin-aws-ce/commit/aee466e749303cc0434270df9955e64e28a99572))
* map Cost Explorer API errors to gRPC codes (CE-6.9) ([6becbf4](https://github.com/rshade/finfocus-plugin-aws-ce/commit/6becbf4f2baad995d848414570205f3cc4aea9d2))
* polish installation and credential setup errors (CE-4.3) ([1a09c42](https://github.com/rshade/finfocus-plugin-aws-ce/commit/1a09c42bebddef6bbaaba5fb11b51d69d32fcde6))
* prefer descriptor identity on spec v0.7.5 (REL-4) ([633637b](https://github.com/rshade/finfocus-plugin-aws-ce/commit/633637b3deb39d6cebf24c00700c330f42669525))
* **pricing:** drop always-true rat nil check (CE-6.1) ([fcc7076](https://github.com/rshade/finfocus-plugin-aws-ce/commit/fcc707622951d5906c8944351c6024d8706a9f42))
* query bare instance ids and reject truncated cost pages (CE-6.1) ([2a72b80](https://github.com/rshade/finfocus-plugin-aws-ce/commit/2a72b8002c95c0b7367a40dd4a02f52607288e14))
* redact assume-role errors and cache the provider (CE-1.6) ([4e69598](https://github.com/rshade/finfocus-plugin-aws-ce/commit/4e695984532debd9ed3df0c46af08fada27ac9c1))
* **release:** copy family release workflows (REL-2) ([5f23540](https://github.com/rshade/finfocus-plugin-aws-ce/commit/5f23540e456fdd3a8d5873ff4d81635485ce0e54))
* **release:** guard family release settings (REL-1) ([ce7b040](https://github.com/rshade/finfocus-plugin-aws-ce/commit/ce7b040e24eea9fee63861ff422adf47135df6d5))
* **release:** validate archive-only snapshots (REL-3) ([139f2d0](https://github.com/rshade/finfocus-plugin-aws-ce/commit/139f2d09bbcc8c78030f4cda324c37bb099c16b3))
* resolve repository markdown lint failures ([c1c0d82](https://github.com/rshade/finfocus-plugin-aws-ce/commit/c1c0d825856a7f6708baaddc13fea591a5c0e8c4))
* serialize client initialization and configure batches (CE-6.6) ([1054ec5](https://github.com/rshade/finfocus-plugin-aws-ce/commit/1054ec5d591846c0f11a6d4f11425ec953e316c6))
* sum service and account costs as decimals (CE-6.2) ([a9db440](https://github.com/rshade/finfocus-plugin-aws-ce/commit/a9db440af23b865d9d84e2189c5cd79a794e46a7))
* test startup configuration and port precedence (CE-2.2) ([a8b45eb](https://github.com/rshade/finfocus-plugin-aws-ce/commit/a8b45ebdf857587e912d59b4162c3f4224b14a38))
* validate and document request credential keys (CE-6.11) ([59a97dd](https://github.com/rshade/finfocus-plugin-aws-ce/commit/59a97dd854e33dc7eecd315104ebbd1aa27d314f))


### Documentation

* format live-check environment name (CE-R.5, CE-R.7) ([abca0f3](https://github.com/rshade/finfocus-plugin-aws-ce/commit/abca0f308d903d305331c60ae790f39c6f379368))
* initial constitution and speckit setup ([2387456](https://github.com/rshade/finfocus-plugin-aws-ce/commit/23874560a82e1b54418805f49c09345ee8fba0fd))
* lint follow-up task descriptions (CE-R.5, CE-R.6, CE-R.7, CE-R.8) ([0db930a](https://github.com/rshade/finfocus-plugin-aws-ce/commit/0db930a62c6d538ba01b5248317ad53658e041f3))
* resolve Vale findings in pull request prose ([ddc784e](https://github.com/rshade/finfocus-plugin-aws-ce/commit/ddc784e5b85627a08ac70c449452f51afd12f51f))
