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

type Repo struct {
	Owner string
	Name  string
}

type Ignore struct {
	owners map[string]struct{}
	names  map[string]struct{}
	repos  map[Repo]struct{}
}

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

func updateFile(file *os.File, text string) {
	data, err := io.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	orig := string(data)
	start := "<!--START_SECTION:prlist-->"
	end := "<!--END_SECTION:prlist-->"
	wrapped := start + "\n" + text + "\n" + end

	if strings.Contains(orig, start) && strings.Contains(orig, end) {
		re := regexp.MustCompile(`(?sm)^` + start + `.*?` + end)
		result := re.ReplaceAllString(orig, wrapped)
		file.Seek(0, 0)
		_, err := io.Copy(file, bytes.NewBuffer([]byte(result)))
		if err != nil {
			log.Fatal(err)
		}
		return
	}
}

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

func reposToHTML(user string, repos []Repo) string {
	tmpl := `
{{$user := .user}}
<ul>
	{{ range .repos }}
  <li> <a href="{{prlink $user . }}">{{ .Owner }}/{{ .Name }}</a> </li>
	{{ end }}
</ul>
	`
	return renderHTMLTemplate(tmpl, user, repos)
}

func reposToBrHTML(user string, repos []Repo) string {
	tmpl := `
{{$user := .user}}
{{ range .repos }}
<a href="{{prlink $user . }}">{{ .Owner }}/{{ .Name }}</a> <br>
{{ end }}
	`
	return renderHTMLTemplate(tmpl, user, repos)
}

func reposToMd(user string, repos []Repo) string {
	list := ""
	for i, repo := range repos {
		list += fmt.Sprintf(
			"- [%s/%s](%s)",
			repo.Owner,
			repo.Name,
			getLinkToPRs(user, repo),
		)
		if i < len(repos)-1 {
			list += "\n"
		}
	}
	return list
}

func getLinkToPRs(user string, repo Repo) string {
	// https://github.com/rpgp/rpgp/pulls?q=is%3Apr%20author%3Aasciimoth
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
