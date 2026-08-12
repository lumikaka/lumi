package httpapi

import (
	"context"
	"errors"
	"net/http"

	"lumi/internal/directorypicker"

	"github.com/labstack/echo/v4"
)

type DirectoryPicker interface {
	Pick(context.Context, string) (string, error)
}

type DirectorySelectionHandler struct {
	picker DirectoryPicker
}

func NewDirectorySelectionHandler(picker DirectoryPicker) *DirectorySelectionHandler {
	return &DirectorySelectionHandler{picker: picker}
}

type createDirectorySelectionRequest struct {
	InitialPath string `json:"initial_path"`
}

type directorySelectionData struct {
	RootPath string `json:"root_path"`
}

func (handler *DirectorySelectionHandler) Create(c echo.Context) error {
	var request createDirectorySelectionRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	selected, err := handler.picker.Pick(c.Request().Context(), request.InitialPath)
	if errors.Is(err, directorypicker.ErrCancelled) {
		return Success(c, http.StatusOK, nil)
	}
	if errors.Is(err, directorypicker.ErrBusy) {
		return NewError(http.StatusConflict, "directory_picker_busy", "文件夹选择器已打开", "请先完成或取消当前文件夹选择。", err)
	}
	if errors.Is(err, directorypicker.ErrUnavailable) {
		return NewError(http.StatusNotImplemented, "directory_picker_unavailable", "当前系统无法打开文件夹选择器", "请手动输入项目文件夹的绝对路径。", err)
	}
	if err != nil {
		return NewError(http.StatusInternalServerError, "directory_picker_failed", "无法打开文件夹选择器", "请重试，或手动输入项目文件夹的绝对路径。", err)
	}
	return Success(c, http.StatusCreated, directorySelectionData{RootPath: selected})
}
