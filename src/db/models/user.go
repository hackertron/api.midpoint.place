package models

import "gorm.io/gorm"

type User struct {
	gorm.Model
	ID          uint    `gorm:"primaryKey;autoIncrement"`
	Email       string  `gorm:"not null;unique;uniqueIndex"`
	DisplayName string  `gorm:"not null"`
	Password    string  `gorm:"not null"`
	Latitude    float64 `gorm:"type:decimal(10,8);not null;default:0"`
	Longitude   float64 `gorm:"type:decimal(11,8);not null;default:0"`
}

func (User) TableName() string {
	return "users"
}
