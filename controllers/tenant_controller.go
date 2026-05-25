package controllers

import (
	"net/http"
	"strconv"

	"workflow-engine/database"
	"workflow-engine/models"
	"workflow-engine/utils"

	"github.com/gin-gonic/gin"
)

type RegisterTenantRequest struct {
	TenantName string `json:"tenant_name" binding:"required"`
	AdminEmail string `json:"admin_email" binding:"required,email"`
	AdminPass  string `json:"admin_password" binding:"required,min=6"`
}

func RegisterTenant(c *gin.Context) {
	var req RegisterTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create Tenant
	tenant := models.Tenant{
		Name: req.TenantName,
	}

	tx := database.DB.Begin()

	if err := tx.Create(&tenant).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tenant"})
		return
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(req.AdminPass)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// Create Admin User for this Tenant
	user := models.User{
		TenantID:     tenant.ID,
		Email:        req.AdminEmail,
		PasswordHash: hashedPassword,
		Role:         "ADMIN",
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create admin user. Email might be in use."})
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"message":   "Tenant and Admin User created successfully",
		"tenant_id": tenant.ID,
		"user_id":   user.ID,
	})
}

func ListTenants(c *gin.Context) {
	var tenants []models.Tenant
	if err := database.DB.Find(&tenants).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tenants"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": tenants,
	})
}

type RegisterUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role" binding:"required"` // 'ADMIN', 'EDITOR', 'VIEWER'
}

func RegisterUser(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")

	var req RegisterUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Role != "ADMIN" && req.Role != "EDITOR" && req.Role != "VIEWER" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user := models.User{
		TenantID:     tenantID.(string),
		Email:        req.Email,
		PasswordHash: hashedPassword,
		Role:         req.Role,
	}

	if err := database.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user. Email might be in use."})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully",
		"user_id": user.ID,
	})
}

type UserTenantResult struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	CreatedAt  string `json:"created_at"`
}

func ListUserTenants(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	var totalCount int64
	database.DB.Table("users").
		Where("users.tenant_id = ?", tenantID).
		Count(&totalCount)
	totalPage := 1
	if limit > 0 {
		totalPage = int((totalCount + int64(limit) - 1) / int64(limit))
	}

	var results []UserTenantResult
	database.DB.Table("users").
		Select("users.id as user_id, users.email, users.role, tenants.id as tenant_id, tenants.name as tenant_name, users.created_at").
		Joins("left join tenants on users.tenant_id = tenants.id").
		Where("users.tenant_id = ?", tenantID).
		Order("users.created_at desc").
		Offset(offset).Limit(limit).
		Scan(&results)

	c.JSON(http.StatusOK, gin.H{
		"data":       results,
		"page":       page,
		"limit":      limit,
		"total_page": totalPage,
	})
}

func DeleteUser(c *gin.Context) {
	tenantID, _ := c.Get("tenant_id")
	userID := c.Param("id")

	var user models.User
	if err := database.DB.Where("id = ? AND tenant_id = ?", userID, tenantID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found or does not belong to your tenant"})
		return
	}

	// Prevent deleting yourself
	requestUserID, exists := c.Get("user_id")
	if exists && requestUserID == user.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot delete your own account"})
		return
	}

	if err := database.DB.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User deleted successfully",
	})
}
