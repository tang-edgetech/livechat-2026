package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/presence"
)

// DashboardSummaryHandler implements the landing-page stats from
// overview.md §6.0/§9.1: Online Agents and Active Chats are live (backed
// by the same WebSocket presence used everywhere else), Entries/
// Records/Traffic/Bot Chats are periodic aggregates the frontend polls
// every ~60s rather than pushes — see §9.1's confirmed metric
// definitions (moved there from the former §12 open item).
func DashboardSummaryHandler(state *appstate.State, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		merchantIDs, err := scopedMerchantIDs(conn, role, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if len(merchantIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"onlineAgents": 0, "activeChats": 0, "entries": 0,
				"records": 0, "traffic": 0, "merchants": 0, "botChats": 0,
			})
			return
		}

		days, _ := strconv.Atoi(c.DefaultQuery("days", "1"))
		if days < 1 {
			days = 1
		}
		placeholders, args := int64SliceToPlaceholders(merchantIDs)

		var activeChats, botChats, entries, records int
		conn.QueryRow(`SELECT COUNT(*) FROM chat WHERE status = 'active' AND merchant_id IN (`+placeholders+`)`, args...).Scan(&activeChats)
		conn.QueryRow(`SELECT COUNT(*) FROM chat WHERE status = 'bot' AND merchant_id IN (`+placeholders+`)`, args...).Scan(&botChats)
		conn.QueryRow(
			`SELECT COUNT(*) FROM chat WHERE merchant_id IN (`+placeholders+`) AND started_at >= DATE_SUB(NOW(), INTERVAL ? DAY)`,
			append(append([]any{}, args...), days)...,
		).Scan(&entries)
		conn.QueryRow(
			`SELECT COUNT(*) FROM visitor WHERE merchant_id IN (`+placeholders+`) AND merged_into_id IS NULL`,
			args...,
		).Scan(&records)

		var traffic int
		conn.QueryRow(
			`SELECT COUNT(*) FROM message msg JOIN chat c ON c.id = msg.chat_id
			 WHERE c.merchant_id IN (`+placeholders+`) AND msg.created_at >= DATE_SUB(NOW(), INTERVAL ? DAY)`,
			append(append([]any{}, args...), days)...,
		).Scan(&traffic)

		onlineAgents, err := onlineAgentCount(context.Background(), conn, redisClient, merchantIDs)
		if err != nil {
			onlineAgents = 0
		}

		c.JSON(http.StatusOK, gin.H{
			"onlineAgents": onlineAgents,
			"activeChats":  activeChats,
			"entries":      entries,
			"records":      records,
			"traffic":      traffic,
			"merchants":    len(merchantIDs),
			"botChats":     botChats,
		})
	}
}

// onlineAgentCount intersects Redis-tracked presence with "is actually an
// Agent on one of these merchants" — presence itself is role-agnostic
// (any staff connection marks itself online), scoping happens here.
func onlineAgentCount(ctx context.Context, conn *sql.DB, redisClient *redis.Client, merchantIDs []int64) (int, error) {
	online, err := presence.OnlineUserIDs(ctx, redisClient)
	if err != nil {
		return 0, err
	}
	if len(online) == 0 {
		return 0, nil
	}

	onlinePlaceholders, onlineArgs := int64SliceToPlaceholders(online)
	merchantPlaceholders, merchantArgs := int64SliceToPlaceholders(merchantIDs)
	query := `SELECT COUNT(DISTINCT u.id) FROM user u
	          JOIN role r ON r.id = u.role_id
	          JOIN user_merchant um ON um.user_id = u.id
	          WHERE r.slug = 'agent' AND u.id IN (` + onlinePlaceholders + `) AND um.merchant_id IN (` + merchantPlaceholders + `)`
	args := append(append([]any{}, onlineArgs...), merchantArgs...)

	var count int
	err = conn.QueryRow(query, args...).Scan(&count)
	return count, err
}
