package ginchat

import (
	"net/http"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

type GroupHandler struct {
	chatBiz *business.ChatBusiness
}

func NewGroupHandler(chatBiz *business.ChatBusiness) *GroupHandler {
	return &GroupHandler{chatBiz: chatBiz}
}

// POST /api/v1/groups
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req business.CreateE2EEGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")
	g, err := h.chatBiz.CreateE2EEGroup(c.Request.Context(), userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": g})
}

// POST /api/v1/groups/:id/members
func (h *GroupHandler) AddMember(c *gin.Context) {
	groupID := c.Param("id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group id is required"})
		return
	}

	var req business.AddGroupMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")
	if err := h.chatBiz.AddGroupMember(c.Request.Context(), userID, groupID, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member added"})
}

// GET /api/v1/groups/:id/members
func (h *GroupHandler) ListMembers(c *gin.Context) {
	groupID := c.Param("id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group id is required"})
		return
	}

	userID := c.GetString("user_id")
	members, err := h.chatBiz.ListGroupMembers(c.Request.Context(), userID, groupID)
	if err != nil {
		if err.Error() == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": members})
}
