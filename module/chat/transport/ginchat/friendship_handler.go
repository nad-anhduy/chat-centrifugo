package ginchat

import (
	"net/http"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

type FriendshipHandler struct {
	friendBiz *business.FriendshipBusiness
}

func NewFriendshipHandler(friendBiz *business.FriendshipBusiness) *FriendshipHandler {
	return &FriendshipHandler{friendBiz: friendBiz}
}

type requestFriendReq struct {
	TargetUserID string `json:"target_user_id" binding:"required"`
}

// POST /api/v1/friendships/request
func (h *FriendshipHandler) Request(c *gin.Context) {
	var req requestFriendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	if err := h.friendBiz.RequestFriend(c.Request.Context(), userID, req.TargetUserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "friend request sent"})
}

type manageFriendReq struct {
	RequestID string `json:"request_id" binding:"required"`
}

// POST /api/v1/friendships/accept
func (h *FriendshipHandler) Accept(c *gin.Context) {
	var req manageFriendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	if err := h.friendBiz.AcceptFriendRequest(c.Request.Context(), userID, req.RequestID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "friend request accepted"})
}

// POST /api/v1/friendships/reject
func (h *FriendshipHandler) Reject(c *gin.Context) {
	var req manageFriendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	if err := h.friendBiz.RejectFriendRequest(c.Request.Context(), userID, req.RequestID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "friend request rejected"})
}

// GET /api/v1/friendships/pending
func (h *FriendshipHandler) Pending(c *gin.Context) {
	userID := c.GetString("user_id")

	requests, err := h.friendBiz.GetPendingRequests(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var res []map[string]interface{}
	for _, req := range requests {
		res = append(res, map[string]interface{}{
			"id":           req.ID,
			"requester_id": req.RequesterID,
			"requester": gin.H{
				"username": req.Requester.Username,
			},
			"created_at": req.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}
