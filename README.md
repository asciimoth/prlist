<h1 align="center" id="title">PR's list</h1>

A GitHub Action that automatically updates your README file with list of your pull requests that have been successfully merged.  
Inspired by [TheDanniCraft/activity-log](https://github.com/TheDanniCraft/activity-log)

## Features
- Filter for specific repos/repo owners
- Multiple render formats

## Example
Merged pull requests by Linus Torvalds:
<!--START_SECTION:prlist-->
- [subsurface/libdc](https://github.com/subsurface/libdc/pulls?q=is%3Apr+author%3Atorvalds)
- [subsurface/subsurface](https://github.com/subsurface/subsurface/pulls?q=is%3Apr+author%3Atorvalds)
- [libgit2/libgit2](https://github.com/libgit2/libgit2/pulls?q=is%3Apr+author%3Atorvalds)
<!--END_SECTION:prlist-->

## Usage
### 1. Add Sections to Your README
Include the following placeholders in your `README`:
```md
<!--START_SECTION:prlist-->
<!--END_SECTION:prlist-->
```

### 2. Create the GitHub Workflow File
Create a new file in your repository under `.github/workflows/`, for example, `pr-list.yml`. Add the following content to this file:
```yaml
# .github/workflows/pr-list.yml:

name: Update list of my PRs

on:
  schedule:
    - cron: '0 0 * * *'   # every day at 00:00 UTC
  workflow_dispatch: # Allows manual triggering

jobs:
  prlist:
    runs-on: ubuntu-latest
    permissions:
      # Give the default GITHUB_TOKEN write permission to commit and push the
      # added or changed files to the repository.
      contents: write
    steps:
      - name: Update PRs list in README
        uses: asciimoth/prlist@v0.1.0
        with:
          ignore: "owner/repo:owner/*:*/repo"
          format: "md"
```

## Inputs
### ignore
`ignore` input defines filter for repos in form of colon-separated list of patterns:  
`owner/repo:owner/*:*/repo`  
- To ignore specific repo with name `a` owned by user `b` use `b/a` pattern
- To ignore all repos by user `c` use `c/*` pattern
- To ignore all repos with name `d` by any user use pattern `*/d`

### format
There is three list formats available:  

`md` (default) produce markdown list of links:
```md
<!--START_SECTION:prlist-->
- [subsurface/libdc](https://github.com/subsurface/libdc/pulls?q=is%3Apr+author%3Atorvalds)
- [subsurface/subsurface](https://github.com/subsurface/subsurface/pulls?q=is%3Apr+author%3Atorvalds)
- [libgit2/libgit2](https://github.com/libgit2/libgit2/pulls?q=is%3Apr+author%3Atorvalds)
<!--END_SECTION:prlist-->
```

`html` produce `<ul>`&`<li>` list:
``` html
<!--START_SECTION:prlist-->
<ul>
<li> <a href="https://github.com/subsurface/libdc/pulls?q=is%3Apr&#43;author%3Atorvalds">subsurface/libdc</a> </li>
<li> <a href="https://github.com/subsurface/subsurface/pulls?q=is%3Apr&#43;author%3Atorvalds">subsurface/subsurface</a> </li>
<li> <a href="https://github.com/libgit2/libgit2/pulls?q=is%3Apr&#43;author%3Atorvalds">libgit2/libgit2</a> </li>
</ul>
<!--END_SECTION:prlist-->
```

GitHub README rendering is pretty simple and usual html list wrapped in centered tag may looks broken.
So there is also the `html-br` format which produce just a set of links separated with `<br>` tags:
```html
<!--START_SECTION:prlist-->
<a href="https://github.com/subsurface/libdc/pulls?q=is%3Apr&#43;author%3Atorvalds">subsurface/libdc</a> <br>
<a href="https://github.com/subsurface/subsurface/pulls?q=is%3Apr&#43;author%3Atorvalds">subsurface/subsurface</a> <br>
<a href="https://github.com/libgit2/libgit2/pulls?q=is%3Apr&#43;author%3Atorvalds">libgit2/libgit2</a> <br>
<!--END_SECTION:prlist-->
```

