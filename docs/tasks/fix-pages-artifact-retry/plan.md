# Make Pages deployments safe to retry

In run 46119581758 the upload succeeded on both attempts. The first deployment found zero matching artifacts immediately after upload. The second found two artifacts named `github-pages` and failed. The run lists two different artifact IDs, 15253580282 and 15254311485. The first result is consistent with delayed visibility. The duplicate arose because retrying the failed job repeated its upload under the same name. [Run history](https://github.com/Yangshifu1024/GitWave/actions/runs/46119581758)

## Changes

Upload the site in a separate job. Name the artifact after the upload attempt and pass that name to the deployment job as an output. GitHub scopes the lookup to this workflow run. Only deployment receives Pages and OIDC write permissions.

## Regression checks

| Scenario | Expected result |
| --- | --- |
| First deployment | Deployment starts after upload succeeds and selects its artifact. |
| The artifact is temporarily invisible | Re-running the failed deployment reuses the successful upload's name and gives the artifact service another chance to find it. |
| Re-run all jobs | The new upload has a different name. Deployment selects it even if an earlier artifact still exists. |

If the uploaded artifact has expired, re-run all jobs to upload a fresh copy. Validate the workflow with actionlint and confirm the next deployment to Pages serves the v0.12.2 site.
