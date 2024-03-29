PORTB = $6000
PORTA = $6001
DDRB = $6002
DDRA = $6003

E = %10000000
RW = %01000000
RS = %00100000

        .org $8000

	;; the reset vector in our memory map is always 0x8000
reset:
        lda #%11111111          ; Set all pins on port B to output
        sta DDRB

        lda #%11100000          ; Set top 3 pins on port A to output
        sta DDRA

        lda #%00111000          ; Set 8-bit mode; 2-line display; 5x8 font
        sta PORTB

        lda #0
        sta PORTA               ; Clear RS/RW/E bits

        lda #E                  ; Set E bit to send instruction
        sta PORTA

        lda #0
        sta PORTA               ; Clear RS/RW/E bits

        lda #%00001110          ; Display on; cursor on; blink off
        sta PORTB

        lda #0
        sta PORTA               ; Clear RS/RW/E bits

        lda #E                  ; Set E bit to send instruction
        sta PORTA

        lda #0
        sta PORTA               ; Clear RS/RW/E bits

        lda #%00000110          ; Increment and shift cursor; don't shift display
        sta PORTB

        lda #0
        sta PORTA               ; Clear RS/RW/E bits

        lda #E                  ; Set E bit to send instruction
        sta PORTA

        lda #0
        sta PORTA               ; Clear RS/RW/E bits

        ldx #(hello_string&255) ; Get the offset from memory 0x8000 for the string
        jsr print_string
        jmp loop


print_string:
        lda $8000,X             ; Assumes the provided index is offset from 0x8000
        cmp #0                  ; Compare the value to zero
        beq end
        jsr print_char
        inx
        jmp print_string
end:
        rts

print_char:
        sta PORTB
        lda #RS
        sta PORTA               ; Clear RS/RW/E bits
        lda #(RS | E)           ; Set E bit to send instruction
        sta PORTA
        lda #RS
        sta PORTA               ; Clear RS/RW/E bits
        rts

loop:
        jmp loop

hello_string:
        ascii "Hello, World!"
