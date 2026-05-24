package controllers

import (
	"net/http"

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
