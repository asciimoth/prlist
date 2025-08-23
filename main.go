// Prlist updates a target file by replacing the content between
// <!--START_SECTION:prlist--> and <!--END_SECTION:prlist--> with a list
// of repositories that have merged PRs authored by a specified GitHub user.
//
// Usage (flags):
// -file (string) target file to update (required)
// -user (string) GitHub username to search PRs for (required)
// -ignore (string) colon-separated ignore list (e.g. "org/*:*/repo")
// -format (string) output format: "md" (default), "html", or "html-br"
//
// Example:
// go run . -file README.md -user torvalds -ignore "org/*:*/repo" -format md
package main

import (
	"bytes"
	"cmp"
	"context"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"maps"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/google/go-github/v74/github"
)

func main() {
	fileName, user, ignoreStr, format := getArgs()
	ctx := context.Background()
	block := IgnorFromString(ignoreStr)
	found := findPRs(ctx, user, &block)
	file, err := os.OpenFile(fileName, os.O_RDWR, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	text := ""
	switch format {
	case "md":
		text = reposToMd(user, found)
	case "html":
		text = reposToHTML(user, found)
	case "html-br":
		text = reposToBrHTML(user, found)
	}
	updateFile(file, text)
}

// getArgs parses CLI flags and returns file, user, ignore and format values.
func getArgs() (string, string, string, string) {
	file := flag.String("file", "", "target file")
	user := flag.String("user", "", "github user")
	ignore := flag.String("ignore", "", "repos to ignore")
	format := flag.String("format", "md", "repos to ignore")
	flag.Parse()
	if *file == "" || *user == "" {
		log.Fatal("Not all cli args are passed")
	}
	return *file, *user, *ignore, *format
}

// Repo identifies a GitHub repository by owner and name.
type Repo struct {
	Owner string
	Name  string
}

// Ignore holds sets of owners, names or explicit repos that should be skipped.
// It is constructed from the -ignore CLI flag.
type Ignore struct {
	owners map[string]struct{}
	names  map[string]struct{}
	repos  map[Repo]struct{}
}

// IgnorFromString parses the ignore string into an Ignore structure. The
// format is a colon-separated list of owner/name pairs. Use "owner/*" to
// ignore all repos of an owner, "*/name" to ignore all repos with that name.
func IgnorFromString(str string) Ignore {
	owners := map[string]struct{}{}
	names := map[string]struct{}{}
	repos := map[Repo]struct{}{}
	pairs := strings.Split(str, ":")
	for _, pair := range pairs {
		elems := strings.Split(pair, "/")
		if len(elems) < 2 {
			continue
		}
		owner := elems[0]
		name := elems[1]
		if owner == "" || name == "" {
			continue
		}
		if owner == "*" {
			names[name] = struct{}{}
			continue
		}
		if name == "*" {
			owners[owner] = struct{}{}
			continue
		}
		repos[Repo{owner, name}] = struct{}{}
	}
	return Ignore{owners, names, repos}
}

// Match reports whether the given repo should be ignored.
func (i *Ignore) Match(repo Repo) bool {
	if i == nil {
		return false
	}
	_, ok := i.repos[repo]
	if ok {
		return true
	}
	_, ok = i.owners[repo.Owner]
	if ok {
		return true
	}
	_, ok = i.names[repo.Name]
	if ok {
		return true
	}
	return false
}

// updateFile replaces the content between markers <!--START_SECTION:prlist--> and
// <!--END_SECTION:prlist--> in the provided file with the supplied text.
func updateFile(file *os.File, text string) {
	data, err := io.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	orig := string(data)
	start := "<!--START_SECTION:prlist-->"
	end := "<!--END_SECTION:prlist-->"
	wrapped := start + "\n" + text + end + "\n"

	if strings.Contains(orig, start) && strings.Contains(orig, end) {
		re := regexp.MustCompile(`(?sm)^` + start + `.*?` + end)
		result := re.ReplaceAllString(orig, wrapped)
		file.Seek(0, 0)
		file.Truncate(0)
		_, err := io.Copy(file, bytes.NewBuffer([]byte(result)))
		if err != nil {
			log.Fatal(err)
		}
		return
	}
}

// renderHTMLTemplate executes a small HTML template with helper functions.
func renderHTMLTemplate(text, user string, repos []Repo) string {
	funcMap := template.FuncMap{
		"prlink": getLinkToPRs,
	}
	tmpl := template.Must(template.New("template").Funcs(funcMap).Parse(text))
	buf := bytes.Buffer{}
	err := tmpl.Execute(&buf, map[string]any{"user": user, "repos": repos})
	if err != nil {
		log.Fatal(err)
	}
	return buf.String()
}

// reposToHTML returns a simple unordered list of repo links in HTML.
func reposToHTML(user string, repos []Repo) string {
	tmpl := "{{$user := .user}}<ul>\n{{ range .repos }}<li> <a href=\"{{prlink $user . }}\">{{ .Owner }}/{{ .Name }}</a> </li>\n{{ end }}</ul>"
	return renderHTMLTemplate(tmpl, user, repos)
}

// reposToBrHTML returns repo links separated by <br> tags.
func reposToBrHTML(user string, repos []Repo) string {
	tmpl := "{{$user := .user}}{{ range .repos }}<a href=\"{{prlink $user . }}\">{{ .Owner }}/{{ .Name }}</a> <br>\n{{ end }}"
	return renderHTMLTemplate(tmpl, user, repos)
}

// reposToMd renders the repositories as Markdown list of links.
func reposToMd(user string, repos []Repo) string {
	list := ""
	for _, repo := range repos {
		list += fmt.Sprintf(
			"- [%s/%s](%s)\n",
			repo.Owner,
			repo.Name,
			getLinkToPRs(user, repo),
		)
	}
	return list
}

// getLinkToPRs builds a GitHub search URL that lists PRs authored by user in repo.
func getLinkToPRs(user string, repo Repo) string {
	// e.g. https://github.com/rpgp/rpgp/pulls?q=is%3Apr%20author%3Aasciimoth
	u := &url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   fmt.Sprintf("/%s/%s/pulls", repo.Owner, repo.Name),
	}
	v := url.Values{}
	v.Set("q", fmt.Sprintf("is:pr author:%s", user))
	u.RawQuery = v.Encode()

	return u.String()
}

// findPRs searches GitHub for merged PRs by the provided user and returns
// a sorted list of unique repositories (most recently merged first). The
// block parameter can be used to exclude certain repos/owners/names.
func findPRs(ctx context.Context, user string, block *Ignore) []Repo {
	client := github.NewClient(nil)
	query := fmt.Sprintf("is:pr author:%s is:merged", user)
	opts := &github.SearchOptions{
		Sort:        "updated",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	found := make(map[Repo]int64)

	for {
		result, resp, err := client.Search.Issues(ctx, query, opts)
		if err != nil {
			log.Fatalf("Search.Issues error: %v", err)
		}

		for _, issue := range result.Issues {
			repoURL := issue.GetRepositoryURL()
			if repoURL == "" {
				continue
			}
			owner, name := ownerRepoFromAPIURL(repoURL)
			if owner == "" || name == "" {
				continue
			}
			repo := Repo{owner, name}
			if block.Match(repo) {
				continue
			}
			time := issue.PullRequestLinks.MergedAt.Unix()
			old, ok := found[repo]
			if ok {
				if time > old {
					found[repo] = time
				}
			} else {
				found[repo] = time
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = resp.NextPage
	}

	sorted := slices.Collect(maps.Keys(found))

	slices.SortFunc(sorted, func(a, b Repo) int {
		return cmp.Compare(found[b], found[a])
	})

	return sorted
}

// ownerRepoFromAPIURL parses "https://api.github.com/repos/owner/repo" into owner, repo
func ownerRepoFromAPIURL(apiURL string) (owner, repo string) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", ""
	}
	// path should be "/repos/owner/repo"
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "repos" {
		return parts[1], parts[2]
	}
	// sometimes API root might omit "repos", handle fallback
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", ""
}
