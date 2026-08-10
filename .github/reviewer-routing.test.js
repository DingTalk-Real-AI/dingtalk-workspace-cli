'use strict';

const assert = require('node:assert/strict');
const {
  requestReviewersWithFallback,
  resolveReviewRouting,
  reviewerCandidates,
} = require('./reviewer-routing');

function route(files, author = 'author', latestPusher = author) {
  return resolveReviewRouting({files: files.map((filename) => ({filename})), author, latestPusher});
}

{
  const result = route(['internal/helpers/chat_toolbar.go']);
  assert.deepEqual(result.reviewers, ['wxianfeng']);
  assert.equal(result.requiredReviewers, 1);
  assert.deepEqual(result.modules.map((module) => module.id), ['product:chat']);
}

{
  const result = route(['internal/helpers/chat_toolbar.go', 'internal/helpers/doc_style.go']);
  assert.deepEqual(result.reviewers, ['wxianfeng', 'typefield']);
  assert.equal(result.requiredReviewers, 2);
}

{
  const result = route(['.github/workflows/ci.yml']);
  assert.deepEqual(result.reviewers, ['haofeng0705', 'wxianfeng']);
  assert.equal(result.requiredReviewers, 2);
  assert.equal(result.reason, 'cross_or_sensitive');
}

{
  const result = route(['internal/auth/login.go'], 'hlzjsong');
  assert.deepEqual(result.reviewers, ['typefield', 'wxianfeng']);
  assert.equal(result.requiredReviewers, 2);
}

{
  const result = route(['internal/upgrade/downloader.go']);
  assert.deepEqual(result.reviewers, ['haofeng0705', 'wxianfeng']);
  assert.equal(result.requiredReviewers, 2);
}

{
  const result = route(['internal/app/upgrade.go', 'scripts/dev/test-release.sh']);
  assert.deepEqual(result.reviewers, ['haofeng0705', 'wxianfeng']);
  assert.equal(result.requiredReviewers, 2);
}

{
  const result = route(['pkg/edition/edition.go']);
  assert.deepEqual(result.reviewers, ['hlzjsong', 'typefield']);
  assert.equal(result.requiredReviewers, 2);
}

{
  const result = route(['internal/shortcut/chat/compatibility_coverage_test.go']);
  assert.deepEqual(result.reviewers, ['wxianfeng', 'typefield']);
  assert.equal(result.requiredReviewers, 2);
  assert.deepEqual(result.modules.map((module) => module.id), ['product:chat', 'compatibility']);
}

{
  const result = route(['internal/helpers/leaf_dispatch.go']);
  assert.deepEqual(result.reviewers, ['wxianfeng', 'typefield']);
  assert.equal(result.requiredReviewers, 2);
}

{
  const result = route(['docs/unknown-area.md']);
  assert.deepEqual(result.reviewers, []);
  assert.equal(result.reason, 'unknown_paths');
}

async function testSingleReviewerFallback() {
  const candidates = reviewerCandidates({
    preferredReviewers: ['wxianfeng'],
    fallbackReviewers: ['wxianfeng', 'typefield', 'haofeng0705'],
    eligibleReviewers: ['wxianfeng', 'typefield', 'haofeng0705'],
  });
  const attempts = [];
  const result = await requestReviewersWithFallback({
    candidates,
    requiredReviewers: 1,
    requestReviewer: async (reviewer) => {
      attempts.push(reviewer);
      if (reviewer === 'wxianfeng') {
        throw Object.assign(new Error('cannot request primary'), {status: 422});
      }
      return true;
    },
  });
  assert.deepEqual(attempts, ['wxianfeng', 'typefield']);
  assert.deepEqual(result.requested, ['typefield']);
  assert.equal(result.satisfiedReviewers.length, 1);
}

async function testTwoReviewerFallback() {
  const candidates = reviewerCandidates({
    preferredReviewers: ['haofeng0705', 'wxianfeng'],
    fallbackReviewers: ['haofeng0705', 'wxianfeng', 'typefield', 'hlzjsong'],
    eligibleReviewers: ['haofeng0705', 'wxianfeng', 'typefield', 'hlzjsong'],
  });
  const attempts = [];
  const result = await requestReviewersWithFallback({
    candidates,
    requiredReviewers: 2,
    requestReviewer: async (reviewer) => {
      attempts.push(reviewer);
      if (reviewer === 'wxianfeng') {
        throw Object.assign(new Error('temporary failure'), {status: 503});
      }
      return true;
    },
  });
  assert.deepEqual(attempts, ['haofeng0705', 'wxianfeng', 'typefield']);
  assert.deepEqual(result.requested, ['haofeng0705', 'typefield']);
  assert.equal(result.satisfiedReviewers.length, 2);
}

async function testLowerPriorityExistingRequestDoesNotReplaceOwner() {
  const attempts = [];
  const result = await requestReviewersWithFallback({
    candidates: ['wxianfeng', 'typefield'],
    requiredReviewers: 1,
    satisfiedReviewers: ['typefield'],
    requestReviewer: async (reviewer) => {
      attempts.push(reviewer);
      return true;
    },
  });
  assert.deepEqual(attempts, ['wxianfeng']);
  assert.deepEqual(result.requested, ['wxianfeng']);
  assert.deepEqual(result.satisfiedReviewers, ['wxianfeng']);
}

Promise.all([
  testSingleReviewerFallback(),
  testTwoReviewerFallback(),
  testLowerPriorityExistingRequestDoesNotReplaceOwner(),
])
  .then(() => console.log('reviewer routing policy tests passed'))
  .catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
