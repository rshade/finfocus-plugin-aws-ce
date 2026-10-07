# Changelog

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
