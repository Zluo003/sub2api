import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import fork from './fork'
import { deepMergeLocale } from '../../deepMerge'

// fork 为本仓库二开新增的键，深合并进上游模块（见 ../../deepMerge.ts）
export default deepMergeLocale(
  {
    ...landing,
    ...common,
    ...dashboard,
    ...batchImage,
    admin,
    ...misc,
  },
  fork,
)
