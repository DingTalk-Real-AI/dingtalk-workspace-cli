'use strict';

// 评审归属是受保护分支上的声明式规则；未知路径不猜测，交给工作流负载均衡兜底。
const REVIEWER_POOL = ['wxianfeng', 'typefield', 'haofeng0705', 'hlzjsong'];

const PRODUCT_GROUPS = [
  {
    primary: 'wxianfeng',
    backup: 'typefield',
    products: ['chat', 'contact', 'ding', 'event', 'mail', 'live', 'conference', 'dev', 'devapp', 'mcp', 'aiapp'],
  },
  {
    primary: 'typefield',
    backup: 'wxianfeng',
    products: ['doc', 'drive', 'wiki', 'markdown', 'docparse', 'aidesign', 'devdoc', 'blackboard', 'finance', 'law', 'credit'],
  },
  {
    primary: 'haofeng0705',
    backup: 'typefield',
    products: ['minutes', 'sheet', 'aitable', 'calendar', 'todo', 'oa', 'attendance', 'report', 'agoal', 'aisearch', 'yida', 'hrbrain'],
  },
];

const pathStartsWith = (prefixes) => (path) => prefixes.some((prefix) => path.startsWith(prefix));

const MODULES = [
  {
    id: 'security',
    label: '登录、认证、权限、安全',
    primary: 'hlzjsong',
    backup: 'typefield',
    requiresSecondary: true,
    matches: pathStartsWith([
      'internal/auth/',
      'internal/keychain/',
      'internal/audit/',
      'internal/pat/',
      'internal/security/',
      'internal/safety/',
      'pkg/edition/',
    ]),
  },
  {
    id: 'delivery',
    label: 'CI、测试、发布、安装',
    primary: 'haofeng0705',
    backup: 'wxianfeng',
    requiresSecondary: true,
    matches: (path) =>
      path.startsWith('.github/') ||
      path.startsWith('scripts/release/') ||
      path.startsWith('scripts/policy/') ||
      path.startsWith('scripts/dev/') ||
      path.startsWith('scripts/install') ||
      path.startsWith('Formula/') ||
      path.startsWith('build/') ||
      path.startsWith('internal/upgrade/') ||
      path.startsWith('internal/app/upgrade') ||
      path.startsWith('test/') ||
      path.startsWith('verify/') ||
      path.startsWith('.workflow/') ||
      path === 'coverage.txt' ||
      path === 'coverage-base.txt' ||
      path === '.goreleaser.yaml' ||
      path === 'package.json' ||
      path === 'package-lock.json' ||
      path === 'docs/releasing.md',
  },
  {
    id: 'architecture',
    label: 'DWS 架构、公共内核',
    primary: 'wxianfeng',
    backup: 'typefield',
    requiresSecondary: true,
    matches: pathStartsWith([
      'cmd/',
      'internal/apiclient/',
      'internal/app/',
      'internal/cli/',
      'internal/cobracmd/',
      'internal/corecmd/',
      'internal/errors/',
      'internal/executor/',
      'internal/generator/',
      'internal/i18n/',
      'internal/interfacesnapshot/',
      'internal/jsonutil/',
      'internal/localio/',
      'internal/logging/',
      'internal/output/',
      'internal/pipeline/',
      'internal/plugin/',
      'internal/profilectx/',
      'internal/registry/',
      'internal/syncdata/',
      'internal/testseam/',
      'internal/transport/',
      'pkg/',
    ]),
  },
  {
    id: 'compatibility',
    label: '兼容性',
    primary: 'wxianfeng',
    backup: 'typefield',
    requiresSecondary: true,
    matches: (path) =>
      /(?:^|[/_.-])compat(?:ibility)?(?=$|[/_.-])/.test(path) ||
      path.includes('schema_compat'),
  },
];

function productMatches(path, product) {
  const aliases = product === 'blackboard' ? ['blackboard', 'whiteboard'] : [product];
  return aliases.some((alias) => new RegExp(`(?:^|[/_.-])${alias}(?=$|[/_.-])`).test(path));
}

const PRODUCT_MODULES = PRODUCT_GROUPS.flatMap((group) =>
  group.products.map((product) => ({
    id: `product:${product}`,
    label: `产品：${product}`,
    primary: group.primary,
    backup: group.backup,
    requiresSecondary: false,
    matches: (path) => productMatches(path, product),
  })),
);

const ALL_MODULES = [MODULES[0], MODULES[1], ...PRODUCT_MODULES, MODULES[2], MODULES[3]];

function normalizedPaths(file) {
  return [file?.filename, file?.previous_filename]
    .filter((path) => typeof path === 'string' && path !== '')
    .map((path) => path.toLowerCase());
}

function compareStats(left, right) {
  return right.files - left.files || left.module.order - right.module.order || left.module.id.localeCompare(right.module.id);
}

function classifyFiles(files) {
  const counts = new Map();
  for (const file of files || []) {
    const matchingModules = new Set();
    for (const path of normalizedPaths(file)) {
      const matches = ALL_MODULES.filter((module) => module.matches(path));
      const securityOrDelivery = matches.filter(
        (module) => module.id === 'security' || module.id === 'delivery',
      );
      const effectiveMatches = securityOrDelivery.length > 0
        ? [...securityOrDelivery, ...matches.filter((module) => module.id === 'compatibility')]
        : matches;
      for (const match of effectiveMatches) {
        matchingModules.add(match.id);
      }
      if (
        effectiveMatches.length === 0 &&
        (path.startsWith('internal/helpers/') || path.startsWith('internal/shortcut/'))
      ) {
        matchingModules.add('architecture');
      }
    }
    for (const moduleID of matchingModules) {
      counts.set(moduleID, (counts.get(moduleID) || 0) + 1);
    }
  }

  return [...counts.entries()]
    .map(([id, files]) => {
      const index = ALL_MODULES.findIndex((module) => module.id === id);
      return {module: {...ALL_MODULES[index], order: index}, files};
    })
    .sort(compareStats);
}

function chooseModuleReviewer(module, unavailable) {
  return [module.primary, module.backup].find(
    (reviewer) => REVIEWER_POOL.includes(reviewer) && !unavailable.has(reviewer),
  );
}

function addReviewer(reviewers, reviewer) {
  if (reviewer && !reviewers.includes(reviewer)) {
    reviewers.push(reviewer);
  }
}

function reviewerCandidates({preferredReviewers, fallbackReviewers, eligibleReviewers}) {
  const eligible = new Set(eligibleReviewers.map((reviewer) => reviewer.toLowerCase()));
  const candidates = [];
  for (const reviewer of [...preferredReviewers, ...fallbackReviewers]) {
    if (
      eligible.has(reviewer.toLowerCase()) &&
      !candidates.some((candidate) => candidate.toLowerCase() === reviewer.toLowerCase())
    ) {
      candidates.push(reviewer);
    }
  }
  return candidates;
}

async function requestReviewersWithFallback({
  candidates,
  requiredReviewers,
  satisfiedReviewers = [],
  requestReviewer,
  onFailure = () => {},
}) {
  const alreadySatisfied = new Set(
    satisfiedReviewers.map((reviewer) => reviewer.toLowerCase()),
  );
  const satisfied = new Set();
  const requested = [];

  for (const reviewer of candidates) {
    if (satisfied.size >= requiredReviewers) {
      break;
    }
    const normalizedReviewer = reviewer.toLowerCase();
    if (alreadySatisfied.has(normalizedReviewer)) {
      satisfied.add(normalizedReviewer);
      continue;
    }

    try {
      const shouldContinue = await requestReviewer(reviewer);
      if (shouldContinue === false) {
        return {requested, satisfiedReviewers: [...satisfied], aborted: true};
      }
      requested.push(reviewer);
      satisfied.add(normalizedReviewer);
    } catch (error) {
      onFailure(reviewer, error);
    }
  }

  return {requested, satisfiedReviewers: [...satisfied], aborted: false};
}

function resolveReviewRouting({files, author, latestPusher, fallbackReviewers = REVIEWER_POOL}) {
  const modules = classifyFiles(files);
  const unavailable = new Set([author, latestPusher].filter(Boolean).map((login) => login.toLowerCase()));
  const reviewers = [];
  const primaryModule = modules[0];

  if (!primaryModule) {
    return {modules: [], reviewers, requiredReviewers: 1, reason: 'unknown_paths'};
  }

  addReviewer(reviewers, chooseModuleReviewer(primaryModule.module, unavailable));

  const requiresSecondary =
    modules.length > 1 || modules.some(({module}) => module.requiresSecondary);
  const secondaryModule = modules.find(({module}) => module.id !== primaryModule.module.id) || primaryModule;
  if (requiresSecondary) {
    addReviewer(
      reviewers,
      chooseModuleReviewer(secondaryModule.module, new Set([...unavailable, ...reviewers])),
    );
  }

  for (const reviewer of fallbackReviewers) {
    if (reviewers.length >= (requiresSecondary ? 2 : 1)) {
      break;
    }
    if (REVIEWER_POOL.includes(reviewer) && !unavailable.has(reviewer)) {
      addReviewer(reviewers, reviewer);
    }
  }

  return {
    modules: modules.map(({module, files}) => ({id: module.id, label: module.label, files})),
    reviewers,
    requiredReviewers: requiresSecondary ? 2 : 1,
    reason: requiresSecondary ? 'cross_or_sensitive' : 'single_module',
  };
}

module.exports = {
  REVIEWER_POOL,
  classifyFiles,
  requestReviewersWithFallback,
  resolveReviewRouting,
  reviewerCandidates,
};
