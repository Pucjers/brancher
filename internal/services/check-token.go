package services

import (
	"brancher/config"
	"brancher/internal/utils"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
)

type GitHubTree struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"tree"`
}

type BranchService struct {
	Config *config.Config
}

func NewBranchService(config *config.Config) *BranchService {
	return &BranchService{Config: config}
}
func (s *BranchService) CreateGitBranch(branchName string, token string) error {
	branchName = s.Config.BranchPrefix + branchName
	headers := map[string]string{
		"Authorization": fmt.Sprintf("token %s", s.Config.GitHubToken),
		"Accept":        "application/vnd.github.v3+json",
	}

	baseBranchURL := fmt.Sprintf("%s/repos/%s/%s/git/ref/heads/main", s.Config.GitHubAPIURL, s.Config.RepoOwner, s.Config.RepoName)
	baseBranchResp, err := utils.MakeRequest("GET", baseBranchURL, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to get base branch: %w", err)
	}

	var baseBranchData map[string]interface{}
	json.Unmarshal(baseBranchResp, &baseBranchData)
	baseSHA := baseBranchData["object"].(map[string]interface{})["sha"].(string)

	createBranchURL := fmt.Sprintf("%s/repos/%s/%s/git/refs", s.Config.GitHubAPIURL, s.Config.RepoOwner, s.Config.RepoName)
	createBranchPayload := map[string]string{
		"ref": fmt.Sprintf("refs/heads/%s", branchName),
		"sha": baseSHA,
	}

	_, err = utils.MakeRequest("POST", createBranchURL, headers, createBranchPayload)
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	treeURL := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", s.Config.GitHubAPIURL, s.Config.RepoOwner, s.Config.RepoName, baseSHA)
	treeResp, err := utils.MakeRequest("GET", treeURL, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch repository tree: %w", err)
	}
	var tree GitHubTree
	json.Unmarshal(treeResp, &tree)

	// for _, item := range tree.Tree {
	// 	if item.Type == "blob" {
	// 		if path.Base(item.Path) == "config.yaml" {
	// 			if err := modifyConfig(item.Path, item.SHA, branchName, token, headers); err != nil {
	// 				return err
	// 			}
	// 		} else {
	// 			if err := copyFile(item.Path, item.SHA, branchName, headers); err != nil {
	// 				return err
	// 			}
	// 		}
	// 	}
	// }

	if err := s.cloneBranchToDirectory(branchName); err != nil {
		return fmt.Errorf("failed to clone branch locally: %w", err)
	}
	dirName := fmt.Sprintf("repo-%s", strings.ReplaceAll(branchName, "/", "_"))
	buildCmd := fmt.Sprintf("cd %s && go build -o main main.go", dirName)

	log.Printf("Building main.go in directory '%s'...", dirName)
	if err := utils.ExecuteCommand(buildCmd); err != nil {
		return fmt.Errorf("failed to build main.go in '%s': %w", dirName, err)
	}

	log.Printf("Successfully built main.go in directory '%s'", dirName)

	return nil
}

func (s *BranchService) cloneBranchToDirectory(branchName string) error {
	dirName := fmt.Sprintf("repo-%s", strings.ReplaceAll(branchName, "/", "_"))

	repoURL := fmt.Sprintf("https://%s@github.com/%s/%s.git", s.Config.GitHubToken, s.Config.RepoOwner, s.Config.RepoName)

	cloneCmd := fmt.Sprintf("git clone -b %s %s %s", branchName, repoURL, dirName)

	log.Printf("Cloning branch '%s' into directory '%s'...", branchName, dirName)

	if err := utils.ExecuteCommand(cloneCmd); err != nil {
		return fmt.Errorf("failed to clone branch '%s': %w", branchName, err)
	}

	log.Printf("Branch '%s' successfully cloned into '%s'", branchName, dirName)
	return nil
}

func (s *BranchService) CreateServiceFile(branchName string) error {
	branchName = s.Config.BranchPrefix + branchName
	serviceFileContent := fmt.Sprintf(`[Unit]
Description=Service for branch %s
After=network.target
StartLimitIntervalSec=10

[Service]
Type=simple
ExecStart=/path/to/executable --branch %s
Restart=always
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
`, branchName, branchName)
	safeBranchName := strings.ReplaceAll(branchName, "/", "_")
	serviceFilePath := fmt.Sprintf("/etc/systemd/system/%s.service", safeBranchName)
	return ioutil.WriteFile(serviceFilePath, []byte(serviceFileContent), 0644)
}

func (s *BranchService) UpdateAndStartService(branchName string) error {
	branchName = s.Config.BranchPrefix + branchName
	safeBranchName := strings.ReplaceAll(branchName, "/", "_")
	cmds := []string{
		"systemctl daemon-reload",
		fmt.Sprintf("systemctl enable %s.service", safeBranchName),
		fmt.Sprintf("systemctl start %s.service", safeBranchName),
	}

	for _, cmd := range cmds {
		if err := utils.ExecuteCommand(cmd); err != nil {
			return err
		}
	}

	return nil
}
