package desktimers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// ── commits on task cards ───────────────────────────────────────────────────
// A commit is told to DeskTick the moment it is made, so a task card's count
// goes up at once (POST /api/github/commits). GitHub's push webhook is what
// confirms it later and what catches commits made anywhere else; the API
// shows one deskgit told it of as "not pushed yet" until a push carries it.
//
// What is sent is what the card shows: where, who, when, the message, and
// for each file its path and how many lines changed. Never a diff: the task
// page reads those from GitHub when somebody opens a file.

const (
	// commitsPerRequest and filesPerCommit are the most the route takes.
	commitsPerRequest = 50
	filesPerCommit    = 300
	// messageBytes is the most of a message the route takes.
	messageBytes = 64 << 10
	// queuedCommitsKept bounds the queue of somebody who never signs in.
	queuedCommitsKept = 500
	// commitQueueDir is in the common git dir, one file a commit: a hook
	// queueing and deskgit sending never write the same file.
	commitQueueDir = "desktimers-commits"
)

// CommitFile is one file a commit changed. Additions and Deletions are nil
// for a binary file.
type CommitFile struct {
	Path         string `json:"path"`
	PreviousPath string `json:"previousPath,omitempty"`
	// Status is git's letter: A, M, D, R (renamed), C (copied) or T (type).
	Status    string `json:"status"`
	Additions *int   `json:"additions"`
	Deletions *int   `json:"deletions"`
}

// Commit is a commit as the route takes it.
type Commit struct {
	// Repo is the origin remote's "owner/name".
	Repo string `json:"repo"`
	Sha  string `json:"sha"`
	// Branch is nil on a detached HEAD.
	Branch      *string `json:"branch"`
	Message     string  `json:"message"`
	AuthorName  string  `json:"authorName"`
	AuthorEmail string  `json:"authorEmail"`
	AuthoredAt  string  `json:"authoredAt"`
	// PickedCode is the task picked in deskgit when the commit was made; the
	// API reads codes from it, the message and the branch alike.
	PickedCode string `json:"pickedCode,omitempty"`
	// A merge is sent without files, and not counted on the card.
	IsMerge bool         `json:"isMerge,omitempty"`
	Files   []CommitFile `json:"files"`
	// FilesTotal is how many files changed when that is more than Files.
	FilesTotal int `json:"filesTotal,omitempty"`
}

// CommitResult is which tasks the API put a commit on. Empty means none: no
// code it knows, or a repository no project of the person's links.
type CommitResult struct {
	Sha       string   `json:"sha"`
	TaskCodes []string `json:"taskCodes"`
}

// SendCommits tells DeskTick of commits, at most commitsPerRequest of them.
func (c *Client) SendCommits(commits []Commit) ([]CommitResult, error) {
	var answer struct {
		Results []CommitResult `json:"results"`
	}
	if err := c.call(http.MethodPost, "/github/commits", map[string]any{"commits": commits}, &answer); err != nil {
		return nil, err
	}
	return answer.Results, nil
}

// ReadCommit reads what the route takes about one commit of the repo at
// repoPath. The branch is the one checked out, which is the commit's own
// only when it is HEAD; it is called for HEAD, right after the commit.
func ReadCommit(repoPath string, rev string) (Commit, error) {
	repo := RepoSlug(repoPath)
	if repo == "" {
		return Commit{}, errors.New("the repository has no origin remote on GitHub")
	}
	// --no-show-signature: log.showSignature would print gpg's check ahead
	// of the format.
	out, err := gitOutput(repoPath, "log", "-1", "--no-show-signature", "--format=%H%x00%P%x00%an%x00%ae%x00%aI%x00%B", rev)
	if err != nil {
		return Commit{}, err
	}
	fields := strings.SplitN(out, "\x00", 6)
	if len(fields) != 6 {
		return Commit{}, fmt.Errorf("reading commit %s: unexpected git output", rev)
	}
	if len(fields[0]) != 40 {
		// A SHA-256 repository: the route takes SHA-1 shas only, and would
		// refuse every commit of it.
		return Commit{}, errors.New("the route takes SHA-1 commits only")
	}
	commit := Commit{
		Repo:        repo,
		Sha:         fields[0],
		Message:     truncateUTF8(strings.TrimRight(fields[5], "\n"), messageBytes),
		AuthorName:  fields[2],
		AuthorEmail: fields[3],
		AuthoredAt:  fields[4],
		IsMerge:     len(strings.Fields(fields[1])) > 1,
		Files:       []CommitFile{},
	}
	if branch := CurrentBranch(repoPath); branch != "" {
		commit.Branch = &branch
	}
	if commit.IsMerge {
		return commit, nil
	}
	files, err := changedFiles(repoPath, commit.Sha)
	if err != nil {
		return Commit{}, err
	}
	if len(files) > filesPerCommit {
		commit.FilesTotal = len(files)
		files = files[:filesPerCommit]
	}
	commit.Files = files
	return commit, nil
}

// changedFiles lists what a commit changed against its parent, or against
// nothing for a root commit. Git lists the same files in the same order for
// --name-status and --numstat; the first says how each changed, the second
// by how many lines.
func changedFiles(repoPath string, sha string) ([]CommitFile, error) {
	args := []string{"diff-tree", "--no-commit-id", "-r", "-M", "--root", "-z"}
	statuses, err := gitOutput(repoPath, append(args, "--name-status", sha)...)
	if err != nil {
		return nil, err
	}
	counts, err := gitOutput(repoPath, append(args, "--numstat", sha)...)
	if err != nil {
		return nil, err
	}

	files := []CommitFile{}
	fields := strings.Split(strings.TrimSuffix(statuses, "\x00"), "\x00")
	for i := 0; i+1 < len(fields) && fields[i] != ""; {
		file := CommitFile{Status: fields[i][:1]}
		if file.Status == "R" || file.Status == "C" {
			if i+2 >= len(fields) {
				break
			}
			file.PreviousPath, file.Path = fields[i+1], fields[i+2]
			i += 3
		} else {
			file.Path = fields[i+1]
			i += 2
		}
		files = append(files, file)
	}

	// --numstat -z: "added\tdeleted\tpath\0", or for a rename or a copy
	// "added\tdeleted\t\0old\0new\0". A binary file counts "-".
	fields = strings.Split(strings.TrimSuffix(counts, "\x00"), "\x00")
	for i, n := 0, 0; i < len(fields) && n < len(files); n++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) != 3 {
			break
		}
		files[n].Additions, files[n].Deletions = lineCount(parts[0]), lineCount(parts[1])
		if parts[2] == "" {
			i += 3
		} else {
			i++
		}
	}
	return files, nil
}

func lineCount(s string) *int {
	var n int
	if _, err := fmt.Sscan(s, &n); err != nil {
		return nil
	}
	return &n
}

// truncateUTF8 cuts s to at most max bytes without splitting a character.
// It looks back no further than one character: a byte that is not UTF-8
// earlier on is the JSON encoder's to replace, not a reason to cut there.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for i := 0; i < utf8.UTFMax-1 && len(s) > 0; i++ {
		if r, size := utf8.DecodeLastRuneInString(s); r != utf8.RuneError || size != 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func gitOutput(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}

// ── the queue ───────────────────────────────────────────────────────────────

// commitQueuePath is in the common git dir, one queue for every worktree of
// a repository: deskgit open in one sends what was committed in another.
func commitQueuePath(repoPath string) (string, error) {
	out, err := gitOutput(repoPath, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(out)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoPath, dir)
	}
	return filepath.Join(dir, commitQueueDir), nil
}

// QueueCommit keeps a commit to be sent. Queueing one twice keeps it once.
func QueueCommit(repoPath string, commit Commit) error {
	dir, err := commitQueuePath(repoPath)
	if err != nil {
		return err
	}
	return queueCommitIn(dir, commit)
}

func queueCommitIn(dir string, commit Commit) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.Marshal(commit)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".queueing-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	if err := os.Rename(tmp.Name(), filepath.Join(dir, commit.Sha+".json")); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	trimCommitQueue(dir)
	return nil
}

type queuedCommit struct {
	path   string
	commit Commit
}

// queuedFiles are the files of the commits waiting, oldest first.
func queuedFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type entry struct {
		path    string
		modTime int64
	}
	waiting := []entry{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		waiting = append(waiting, entry{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	sort.SliceStable(waiting, func(i, j int) bool { return waiting[i].modTime < waiting[j].modTime })
	paths := make([]string, len(waiting))
	for i, e := range waiting {
		paths[i] = e.path
	}
	return paths
}

// queuedCommits are the commits waiting, oldest first. One that cannot be
// read is forgotten.
func queuedCommits(dir string) []queuedCommit {
	out := []queuedCommit{}
	for _, path := range queuedFiles(dir) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var commit Commit
		if json.Unmarshal(data, &commit) != nil || commit.Sha == "" {
			os.Remove(path)
			continue
		}
		out = append(out, queuedCommit{path, commit})
	}
	return out
}

func trimCommitQueue(dir string) {
	waiting := queuedFiles(dir)
	for len(waiting) > queuedCommitsKept {
		os.Remove(waiting[0])
		waiting = waiting[1:]
	}
}

// SendQueuedCommits sends the repo's waiting commits and forgets each one
// DeskTick has taken, or refused as malformed (it would refuse it again).
// What it could not send stays for the next try; ErrUnauthorized says the
// person has to sign in first. Two senders at once may send a commit twice,
// which the API takes as once.
func SendQueuedCommits(repoPath string, client *Client) error {
	dir, err := commitQueuePath(repoPath)
	if err != nil {
		return err
	}
	waiting := queuedCommits(dir)
	for len(waiting) > 0 {
		n := min(len(waiting), commitsPerRequest)
		if err := sendCommitBatch(client, waiting[:n]); err != nil {
			return err
		}
		waiting = waiting[n:]
	}
	return nil
}

// refusesTheWholeBatch is a status the route answers for a request it
// stores none of: a malformed commit in it, or too much at once.
func refusesTheWholeBatch(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusRequestEntityTooLarge
}

func sendCommitBatch(client *Client, batch []queuedCommit) error {
	commits := make([]Commit, len(batch))
	for i, queued := range batch {
		commits[i] = queued.commit
	}
	_, err := client.SendCommits(commits)

	var refused *StatusError
	switch {
	case err == nil:
	case errors.As(err, &refused) && refusesTheWholeBatch(refused.Status) && len(batch) > 1:
		// The route takes all of a request or none of it: halve it until
		// what it refuses is one commit, and send the rest.
		half := len(batch) / 2
		if err := sendCommitBatch(client, batch[:half]); err != nil {
			return err
		}
		return sendCommitBatch(client, batch[half:])
	case errors.As(err, &refused) && refusesTheWholeBatch(refused.Status):
		// One commit malformed or too big on its own: it would be refused
		// again, so it is not kept.
	default:
		return err
	}
	for _, queued := range batch {
		os.Remove(queued.path)
	}
	return nil
}

// QueuedCommits are the repo's commits waiting to be sent, oldest first.
func QueuedCommits(repoPath string) []Commit {
	dir, err := commitQueuePath(repoPath)
	if err != nil {
		return nil
	}
	commits := []Commit{}
	for _, queued := range queuedCommits(dir) {
		commits = append(commits, queued.commit)
	}
	return commits
}
