package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// TagHandler handles knowledge base tag operations.
//
// All KB-access checks (own / org-shared / via shared agent) are now
// performed by the route-level g.KBAccessRead / g.KBAccessWrite
// guards in router.go — the guard rewrites c.Request.Context() to
// carry the effective tenant ID, so handlers below just use
// c.Request.Context() the way they always did.
type TagHandler struct {
	tagService interfaces.KnowledgeTagService
	tagRepo    interfaces.KnowledgeTagRepository
	chunkRepo  interfaces.ChunkRepository
}

// DeleteTagRequest represents the request body for deleting a tag
type DeleteTagRequest struct {
	ExcludeIDs []int64 `json:"exclude_ids"` // Chunk seq_ids to exclude from deletion
}

// NewTagHandler creates a new TagHandler.
func NewTagHandler(
	tagService interfaces.KnowledgeTagService,
	tagRepo interfaces.KnowledgeTagRepository,
	chunkRepo interfaces.ChunkRepository,
) *TagHandler {
	return &TagHandler{tagService: tagService, tagRepo: tagRepo, chunkRepo: chunkRepo}
}

// resolveTagID resolves tag_id parameter which can be either UUID or seq_id (integer).
// Uses tenant from c's context — which the route-level KB-access guard
// has already rewritten to the effective tenant for shared KBs.
func (h *TagHandler) resolveTagID(c *gin.Context) (string, error) {
	return h.resolveTagIDWithCtx(c, c.Request.Context())
}

// resolveTagIDWithCtx resolves tag_id using the given context for tenant.
func (h *TagHandler) resolveTagIDWithCtx(c *gin.Context, ctx context.Context) (string, error) {
	tagIDParam := secutils.SanitizeForLog(c.Param("tag_id"))

	if seqID, err := strconv.ParseInt(tagIDParam, 10, 64); err == nil {
		tenantID := types.MustTenantIDFromContext(ctx)
		tag, err := h.tagRepo.GetBySeqID(ctx, tenantID, seqID)
		if err != nil {
			return "", errors.NewNotFoundError("标签不存在")
		}
		return tag.ID, nil
	}
	return tagIDParam, nil
}

// getChunksBySeqIDs retrieves chunks by their seq_ids.
func (h *TagHandler) getChunksBySeqIDs(ctx context.Context, tenantID uint64, seqIDs []int64) ([]*types.Chunk, error) {
	return h.chunkRepo.ListChunksBySeqID(ctx, tenantID, seqIDs)
}

// ListTags godoc
// @Summary      Get the tag list
// @Description  Gets all tags under a knowledge base along with their statistics
// @Tags         Tag Management
// @Accept       json
// @Produce      json
// @Param        id         path      string  true   "Knowledge base ID"
// @Param        page       query     int     false  "Page number"
// @Param        page_size  query     int     false  "Page size"
// @Param        keyword    query     string  false  "Keyword search"
// @Success      200        {object}  map[string]interface{}  "Tag list"
// @Failure      400        {object}  errors.AppError         "Invalid request parameters"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags [get]
func (h *TagHandler) ListTags(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	var page types.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		logger.Error(ctx, "Failed to bind pagination query", err)
		c.Error(errors.NewBadRequestError("分页参数不合法").WithDetails(err.Error()))
		return
	}

	keyword := secutils.SanitizeForLog(c.Query("keyword"))

	tags, err := h.tagService.ListTags(ctx, kbID, &page, keyword)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tags,
	})
}

type createTagRequest struct {
	Name      string `json:"name"       binding:"required"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// CreateTag godoc
// @Summary      Create a tag
// @Description  Creates a new tag under a knowledge base
// @Tags         Tag Management
// @Accept       json
// @Produce      json
// @Param        id       path      string  true  "Knowledge base ID"
// @Param        request  body      object{name=string,color=string,sort_order=int}  true  "Tag information"
// @Success      200      {object}  map[string]interface{}  "Created tag"
// @Failure      400      {object}  errors.AppError         "Invalid request parameters"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags [post]
func (h *TagHandler) CreateTag(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	var req createTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind create tag payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	tag, err := h.tagService.CreateTag(ctx, kbID,
		secutils.SanitizeForLog(req.Name), secutils.SanitizeForLog(req.Color), req.SortOrder)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"kb_id": kbID,
		})
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tag,
	})
}

type updateTagRequest struct {
	Name      *string `json:"name"`
	Color     *string `json:"color"`
	SortOrder *int    `json:"sort_order"`
}

// UpdateTag godoc
// @Summary      Update a tag
// @Description  Updates tag information
// @Tags         Tag Management
// @Accept       json
// @Produce      json
// @Param        id       path      string  true  "Knowledge base ID"
// @Param        tag_id   path      string  true  "Tag ID (UUID or seq_id)"
// @Param        request  body      object  true  "Tag update information"
// @Success      200      {object}  map[string]interface{}  "Updated tag"
// @Failure      400      {object}  errors.AppError         "Invalid request parameters"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags/{tag_id} [put]
func (h *TagHandler) UpdateTag(c *gin.Context) {
	ctx := c.Request.Context()

	tagID, err := h.resolveTagID(c)
	if err != nil {
		c.Error(err)
		return
	}

	var req updateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind update tag payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	tag, err := h.tagService.UpdateTag(ctx, tagID, req.Name, req.Color, req.SortOrder)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tag_id": tagID,
		})
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tag,
	})
}

// DeleteTag godoc
// @Summary      Delete a tag
// @Description  Deletes a tag; use force=true to force-delete a tag that is still referenced, and content_only=true to delete only the content under the tag while keeping the tag itself
// @Tags         Tag Management
// @Accept       json
// @Produce      json
// @Param        id            path      string              true   "Knowledge base ID"
// @Param        tag_id        path      string              true   "Tag ID (UUID or seq_id)"
// @Param        force         query     bool                false  "Force delete"
// @Param        content_only  query     bool                false  "Delete only the content, keep the tag"
// @Param        body          body      DeleteTagRequest    false  "Delete options"
// @Success      200           {object}  map[string]interface{}  "Deleted successfully"
// @Failure      400           {object}  errors.AppError         "Invalid request parameters"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags/{tag_id} [delete]
func (h *TagHandler) DeleteTag(c *gin.Context) {
	ctx := c.Request.Context()

	tagID, err := h.resolveTagID(c)
	if err != nil {
		c.Error(err)
		return
	}

	force := c.Query("force") == "true"
	contentOnly := c.Query("content_only") == "true"

	var req DeleteTagRequest
	_ = c.ShouldBindJSON(&req)

	var excludeUUIDs []string
	if len(req.ExcludeIDs) > 0 {
		tenantID := types.MustTenantIDFromContext(ctx)
		chunks, err := h.getChunksBySeqIDs(ctx, tenantID, req.ExcludeIDs)
		if err != nil {
			logger.Warnf(ctx, "Failed to resolve exclude_ids: %v", err)
		} else {
			excludeUUIDs = make([]string, len(chunks))
			for i, chunk := range chunks {
				excludeUUIDs[i] = chunk.ID
			}
		}
	}

	if err := h.tagService.DeleteTag(ctx, tagID, force, contentOnly, excludeUUIDs); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tag_id": tagID,
		})
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// NOTE: TagHandler currently exposes CRUD for tags and statistics.
// Knowledge / Chunk tagging is handled via dedicated knowledge and FAQ APIs.
