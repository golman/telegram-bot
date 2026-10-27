package bot

import "testing"

func TestDecodeCallback(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    callbackData
		wantErr bool
	}{
		{name: "кнопка до деплоя", in: "delete_-1001234567890_42", want: callbackData{actionDelete, -1001234567890, 42}},
		{name: "restore", in: "restore_-100_7", want: callbackData{actionRestore, -100, 7}},
		{name: "пустая строка", in: "", wantErr: true},
		{name: "chatID не число", in: "delete_x_1", wantErr: true},
		{name: "messageID не число", in: "delete_1_y", wantErr: true},
		{name: "неизвестное действие", in: "noop_1_2", wantErr: true},
		{name: "лишняя часть", in: "delete_1_2_3", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeCallback(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("decodeCallback(%q) = %+v, ожидалась ошибка", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeCallback(%q): неожиданная ошибка %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("decodeCallback(%q) = %+v, ожидалось %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		d    callbackData
	}{
		{name: "delete", d: callbackData{actionDelete, -1001234567890, 42}},
		{name: "restore", d: callbackData{actionRestore, -100, 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeCallback(encodeCallback(tt.d))
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if got != tt.d {
				t.Fatalf("round trip: %+v, ожидалось %+v", got, tt.d)
			}
		})
	}

	if got := encodeCallback(callbackData{actionDelete, -1001234567890, 42}); got != "delete_-1001234567890_42" {
		t.Fatalf("encodeCallback = %q, ожидалось %q", got, "delete_-1001234567890_42")
	}
}

func TestToggleKeyboard(t *testing.T) {
	tests := []struct {
		name     string
		d        callbackData
		wantText string
		wantData string
	}{
		{name: "delete", d: callbackData{actionDelete, -1001234567890, 42}, wantText: "Не актуально", wantData: "delete_-1001234567890_42"},
		{name: "restore", d: callbackData{actionRestore, -100, 7}, wantText: "Восстановить", wantData: "restore_-100_7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := toggleKeyboard(tt.d)
			if kb == nil || len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 1 {
				t.Fatalf("ожидалась одна кнопка, получено %+v", kb)
			}
			btn := kb.InlineKeyboard[0][0]
			if btn.Text != tt.wantText {
				t.Errorf("текст кнопки = %q, ожидалось %q", btn.Text, tt.wantText)
			}
			if btn.CallbackData != tt.wantData {
				t.Errorf("CallbackData = %q, ожидалось %q", btn.CallbackData, tt.wantData)
			}
		})
	}
}
