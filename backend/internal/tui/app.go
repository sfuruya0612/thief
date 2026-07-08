package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
)

var services = []string{"ec2", "rds", "elasticache", "lambda", "ecs", "ecr", "s3", "iam", "ssm", "cfn", "kinesis", "cloudfront", "elb"}

type fetchResult struct {
	rows     [][]string
	cacheHit bool
	cachedAt time.Time
	err      error
}

type model struct {
	cfg           *config.Config
	profiles      []string
	profileIdx    int
	serviceIdx    int
	rows          [][]string
	cursor        int
	loading       bool
	cacheHit      bool
	cachedAt      time.Time
	err           error
	width, height int
}

// Run starts the bubbletea TUI.
func Run(cfg *config.Config) error {
	profiles, err := awsinternal.ListProfiles()
	if err != nil || len(profiles) == 0 {
		profiles = []string{"default"}
	}

	// Use profile from config if set.
	profileIdx := 0
	if cfg.Profile != "" {
		for i, p := range profiles {
			if p == cfg.Profile {
				profileIdx = i
				break
			}
		}
	}

	m := model{
		cfg:        cfg,
		profiles:   profiles,
		profileIdx: profileIdx,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return m.fetchCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "j", "down":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}

		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}

		case "tab":
			m.profileIdx = (m.profileIdx + 1) % len(m.profiles)
			m.cursor = 0
			m.loading = true
			return m, m.fetchCmd()

		case "]":
			m.serviceIdx = (m.serviceIdx + 1) % len(services)
			m.cursor = 0
			m.loading = true
			return m, m.fetchCmd()

		case "[":
			m.serviceIdx = (m.serviceIdx - 1 + len(services)) % len(services)
			m.cursor = 0
			m.loading = true
			return m, m.fetchCmd()

		case "r":
			m.loading = true
			return m, m.fetchCmd()
		}

	case fetchResult:
		m.loading = false
		m.err = msg.err
		m.rows = msg.rows
		m.cacheHit = msg.cacheHit
		m.cachedAt = msg.cachedAt
	}

	return m, nil
}

func (m model) View() string {
	profile := m.currentProfile()
	service := services[m.serviceIdx]

	cacheStatus := "MISS"
	if m.cacheHit {
		elapsed := time.Since(m.cachedAt).Round(time.Minute)
		cacheStatus = fmt.Sprintf("HIT %s ago", elapsed)
	}

	topBar := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render(fmt.Sprintf(" thief TUI  [profile: %s]  [service: %s]  %s", profile, service, cacheStatus))

	if m.loading {
		return topBar + "\n\n  Loading...\n"
	}
	if m.err != nil {
		return topBar + "\n\n  Error: " + m.err.Error() + "\n"
	}
	if len(m.rows) == 0 {
		return topBar + "\n\n  No resources found.\n"
	}

	var sb strings.Builder
	sb.WriteString(topBar + "\n")

	tableHeight := m.height - 5
	if tableHeight < 1 {
		tableHeight = 10
	}
	start := 0
	if m.cursor >= tableHeight {
		start = m.cursor - tableHeight + 1
	}
	end := start + tableHeight
	if end > len(m.rows) {
		end = len(m.rows)
	}

	for i, row := range m.rows[start:end] {
		line := strings.Join(row, "  ")
		if start+i == m.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render("> " + line)
		} else {
			line = "  " + line
		}
		sb.WriteString(line + "\n")
	}

	statusBar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(" Tab:profile  j/k:row  [/]:service  r:refresh  q:quit")
	sb.WriteString("\n" + statusBar)
	return sb.String()
}

func (m model) currentProfile() string {
	if len(m.profiles) == 0 {
		return "default"
	}
	return m.profiles[m.profileIdx]
}

func (m model) fetchCmd() tea.Cmd {
	profile := m.currentProfile()
	region := m.cfg.Region
	service := services[m.serviceIdx]

	return func() tea.Msg {
		ctx := context.Background()
		rows, err := fetchRows(ctx, profile, region, service)
		return fetchResult{rows: rows, err: err, cachedAt: time.Now()}
	}
}

// fetchRows calls the appropriate list function for the given service.
func fetchRows(ctx context.Context, profile, region, service string) ([][]string, error) {
	switch service {
	case "ec2":
		items, err := awsinternal.ListEC2Resources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "rds":
		items, err := awsinternal.ListRDSResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "elasticache":
		items, err := awsinternal.ListElastiCacheResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "lambda":
		items, err := awsinternal.ListLambdaResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "ecs":
		items, err := awsinternal.ListECSResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "ecr":
		items, err := awsinternal.ListECRResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "s3":
		items, err := awsinternal.ListS3Resources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "iam":
		items, err := awsinternal.ListIAMResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "ssm":
		items, err := awsinternal.ListSSMParameters(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "cfn":
		items, err := awsinternal.ListCFNStacks(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "kinesis":
		items, err := awsinternal.ListKinesisResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "cloudfront":
		items, err := awsinternal.ListCloudFrontResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	case "elb":
		items, err := awsinternal.ListELBResources(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		rows := make([][]string, len(items))
		for i, it := range items {
			rows[i] = it.ToRow()
		}
		return rows, nil
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}
}
