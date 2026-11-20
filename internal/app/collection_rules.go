package app

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// collectionMultiEpisode 识别常见多集与范围标记，不能误当作一个确定单集。
var collectionMultiEpisode = regexp.MustCompile(`(?i)(e\d+\s*[-~+&,]?\s*e\d+|s\d+e\d+\s*[-~+]\s*\d+|\d+x\d+\s*[-~+]\s*\d+|第\d+\s*[-~至]\s*\d+集)`)

// collectionRuleMatch 对关键词、排除、画质和大小执行明确的确定性匹配。
func collectionRuleMatch(rule CollectionRule, c CollectionCandidate) (bool, string) {
	title := strings.ToLower(c.Title)
	for _, word := range strings.Split(rule.Include, ",") {
		word = strings.TrimSpace(strings.ToLower(word))
		if word != "" && !strings.Contains(title, word) {
			return false, "未包含全部关键词"
		}
	}
	for _, word := range strings.Split(rule.Exclude, ",") {
		word = strings.TrimSpace(strings.ToLower(word))
		if word != "" && strings.Contains(title, word) {
			return false, "命中排除关键词"
		}
	}
	if rule.Quality != "" && !strings.Contains(title, strings.ToLower(rule.Quality)) {
		return false, "画质条件不匹配"
	}
	if rule.MinBytes > 0 && (c.SizeBytes == 0 || c.SizeBytes < rule.MinBytes) || rule.MaxBytes > 0 && (c.SizeBytes == 0 || c.SizeBytes > rule.MaxBytes) {
		return false, "体积未知或不在规则范围"
	}
	return true, "命中明确关键词、排除词、画质与大小约束"
}

// collectionEpisodeKey 只接受显式 season/episode 命名捕获，绝对集数和季包保持人工确认。
func collectionEpisodeKey(pattern string, title string) string {
	if pattern == "" || collectionMultiEpisode.MatchString(title) {
		return ""
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	matches := expression.FindAllStringSubmatch(title, 2)
	if len(matches) != 1 {
		return ""
	}
	seasonIndex := expression.SubexpIndex("season")
	episodeIndex := expression.SubexpIndex("episode")
	if seasonIndex < 1 || episodeIndex < 1 {
		return ""
	}
	season, e1 := strconv.Atoi(matches[0][seasonIndex])
	episode, e2 := strconv.Atoi(matches[0][episodeIndex])
	if e1 != nil || e2 != nil || season < 0 || season > 999 || episode < 1 || episode > 9999 {
		return ""
	}
	lower := strings.ToLower(title)
	for _, marker := range []string{"complete", "season pack", "batch", "合集", "全集", "全季"} {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	return strconv.Itoa(season) + ":" + strconv.Itoa(episode)
}

// collectionTakenEpisodes 分页检查全部持久获取历史，不用最新候选窗口推断未获取。
func (a *App) collectionTakenEpisodes(ctx context.Context, s CollectionSubscription) (map[string]bool, error) {
	taken := map[string]bool{}
	after := int64(0)
	for {
		rows, err := a.db.QueryContext(ctx, "SELECT rowid,title FROM collection_tasks WHERE kind='bt' AND rowid>? ORDER BY rowid LIMIT 200", after)
		if err != nil {
			return nil, err
		}
		count := 0
		for rows.Next() {
			var title string
			if err = rows.Scan(&after, &title); err != nil {
				_ = rows.Close()
				return nil, err
			}
			count++
			candidate := CollectionCandidate{Title: title}
			identityRule := s.Rule
			identityRule.Quality = ""
			identityRule.MinBytes = 0
			identityRule.MaxBytes = 0
			if matched, _ := collectionRuleMatch(identityRule, candidate); matched {
				if key := collectionEpisodeKey(s.Rule.EpisodePattern, title); key != "" {
					taken[key] = true
				}
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
		if count < 200 {
			return taken, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

// evaluateCollectionSubscription 默认保留待确认，仅对唯一季集和唯一规则授权自动登记。
func (a *App) evaluateCollectionSubscription(ctx context.Context, s CollectionSubscription) error {
	if !s.Enabled {
		return nil
	}
	source, err := a.collectionSourceByID(ctx, s.SourceID)
	if err != nil || !source.Enabled {
		return nil
	}
	candidates, err := a.collectionCandidates(ctx, s.SourceID, 100)
	if err != nil {
		return err
	}
	matched := []CollectionCandidate{}
	episodes := map[string]int{}
	taken, err := a.collectionTakenEpisodes(ctx, s)
	if err != nil {
		return err
	}
	otherRules, _ := a.collectionSubscriptions(ctx)
	for _, candidate := range candidates {
		if candidate.TaskID != "" {
			if match, _ := collectionRuleMatch(s.Rule, candidate); match {
				key := collectionEpisodeKey(s.Rule.EpisodePattern, candidate.Title)
				if key != "" {
					taken[key] = true
				}
			}
		}
		if candidate.Decision != "review" || candidate.TaskID != "" {
			continue
		}
		if match, _ := collectionRuleMatch(s.Rule, candidate); match {
			matched = append(matched, candidate)
			key := collectionEpisodeKey(s.Rule.EpisodePattern, candidate.Title)
			if key != "" {
				episodes[key]++
			}
		}
	}
	for _, c := range matched {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := collectionEpisodeKey(s.Rule.EpisodePattern, c.Title)
		reason := "命中订阅，等待人工确认"
		matchingRules := 0
		for _, other := range otherRules {
			if other.SourceID == s.SourceID && other.Enabled && other.Mode == "automatic" {
				if match, _ := collectionRuleMatch(other.Rule, c); match {
					matchingRules++
				}
			}
		}
		automatic := s.Mode == "automatic" && key != "" && episodes[key] == 1 && !taken[key] && matchingRules == 1
		if s.Mode == "automatic" && !automatic {
			reason = "作品或季集不明确、存在多个发行或绝对集数未映射，等待确认"
		}
		if automatic {
			request, e := newDownloadUUID()
			if e != nil {
				return e
			}
			_, e = a.queueCollectionBT(ctx, c, request, s.ContentType, s.CreateLibrary, s.LibraryID, s.SelectionPattern, s)
			if e == nil {
				continue
			}
			reason = "自动获取未能登记，请检查目标下载库和来源后人工确认"
		}
		a.collections.mu.Lock()
		latest, e := a.collectionCandidateByID(ctx, c.ID)
		if e == nil && latest.Decision == "review" && latest.TaskID == "" && latest.Reason != reason {
			latest.Reason = reason
			latest.Revision++
			_, _ = a.db.ExecContext(ctx, "UPDATE collection_candidates SET body=? WHERE id=?", collectionJSON(latest), latest.ID)
		}
		a.collections.mu.Unlock()
	}
	return nil
}

// scheduleCollectionRSS 按持久化下一时间调度并退避失败，停用来源不会继续发现新任务。
func (a *App) scheduleCollectionRSS(ctx context.Context) {
	subscriptions, err := a.collectionSubscriptions(ctx)
	if err != nil {
		return
	}
	sources, err := a.collectionSources(ctx)
	if err != nil {
		return
	}
	sourceMap := map[string]CollectionSource{}
	for _, s := range sources {
		sourceMap[s.ID] = s
	}
	refreshed := map[string]error{}
	for _, sub := range subscriptions {
		if ctx.Err() != nil {
			return
		}
		if !sub.Enabled || !collectionExpired(sub.NextRunAt) {
			continue
		}
		source, ok := sourceMap[sub.SourceID]
		if !ok || !source.Enabled {
			continue
		}
		fetchErr, done := refreshed[source.ID]
		if !done {
			_, fetchErr = a.refreshCollectionRSS(ctx, source, false)
			refreshed[source.ID] = fetchErr
		}
		if fetchErr == nil {
			_ = a.evaluateCollectionSubscription(ctx, sub)
		}
		minutes := source.RefreshMinutes
		if fetchErr != nil && minutes < 5 {
			minutes = 5
		}
		a.collections.mu.Lock()
		var body string
		if a.db.QueryRowContext(ctx, "SELECT body FROM collection_subscriptions WHERE id=?", sub.ID).Scan(&body) == nil {
			var latest CollectionSubscription
			if json.Unmarshal([]byte(body), &latest) == nil && latest.Revision == sub.Revision {
				latest.NextRunAt = time.Now().UTC().Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339Nano)
				_, _ = a.db.ExecContext(ctx, "UPDATE collection_subscriptions SET body=? WHERE id=?", collectionJSON(latest), latest.ID)
			}
		}
		a.collections.mu.Unlock()
	}
}
