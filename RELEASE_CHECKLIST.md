# Multicluster Global Hub Release Checklist

## Release Branch Preparation
- [ ] **Create a new branch for multicluster-global-hub repo**
  - Create new release branch for multicluster-global-hub repository
- [ ] **Update pipelinesascode.tekton.dev/on-cel-expression** in `.tekton/` files to point to the correct release branch
  - Ensure `target_branch == "release-X.Y"` matches the actual release branch name
- [ ] **Bump version to new version in main branch** after release
  - Update version files for next development cycle
- [ ] **Check the Containerfile.* to update the version**
  - Update version references in all Containerfile.* files
- [ ] **Create a new branch for glo-grafana and postgres_exporter repo**
  - Create new release branch for glo-grafana repository
  - Create new release branch for postgres_exporter repository

## Catalog Update
- [ ] **Create a new release branch based on the previous latest available release branch**
  - Create new release branch for catalog repository
  - Update supported OCP versions
- [ ] **Update multicluster-global-hub-operator-catalog main branch** to adopt the new release
  - Add new release to catalog repository
- [ ] **Update readme.md file**
  - Update documentation with new release information
- [ ] **Update images_mirror_set.yaml to point to new release version**
  - Update image references and digests for new release
- [ ] **Update GitHub Action to add /lgtm and /approve labels for new release and stop the old one**
  - Configure automation for new release branch
  - Disable automation for previous release branch

## Bundle Update
- [ ] **Create a new release branch based on the previous latest available release branch**
  - Create new release branch for bundle repository
- [ ] **Update bundle to point to current release version**
  - Update version references in bundle manifests
- [ ] **Update bundle folder with latest manifests**
  - Regenerate and validate bundle contents

## Release Data Update
- [ ] **Update konflux-release-data repo to add for new release**
  - Add new release configuration to konflux-release-data repository