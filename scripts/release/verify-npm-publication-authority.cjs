"use strict";

// Called from trusted tooling in each publisher attempt. Artifact integrity is
// necessary but does not authorize republishing a version withdrawn since prep.
module.exports = async function verifyPublication({ github, context, version, commit, tagObject }) {
  if (!/^v\d+\.\d+\.\d+(?:-beta\.[1-9]\d*)?$/.test(version) ||
      !/^[a-f0-9]{40}$/.test(commit) || !/^[a-f0-9]{40}$/.test(tagObject)) {
    throw new Error("Missing or invalid sealed publication identity");
  }
  const repo = context.repo;
  try {
    await github.rest.git.getRef({ ...repo, ref: `tags/withdrawn/${version}` });
    throw new Error(`${version} was withdrawn and cannot be published`);
  } catch (error) {
    if (error.status !== 404) throw error;
  }
  const { data: ref } = await github.rest.git.getRef({ ...repo, ref: `tags/${version}` });
  if (ref.object.type !== "tag" || ref.object.sha !== tagObject) {
    throw new Error("Release tag object changed after preparation");
  }
  const { data: tag } = await github.rest.git.getTag({ ...repo, tag_sha: tagObject });
  if (tag.tag !== version || tag.object.type !== "commit" || tag.object.sha !== commit) {
    throw new Error("Release tag no longer identifies the prepared commit");
  }
  const { data: release } = await github.rest.repos.getReleaseByTag({ ...repo, tag: version });
  if (release.tag_name !== version || release.draft || release.immutable !== true ||
      release.prerelease !== version.includes("-beta.")) {
    throw new Error("Release is no longer public, immutable, and in the sealed channel");
  }
};
