#!/usr/bin/env python3
"""
Generate OdooNoir app icon - Modern, dark theme with green accent.
Designed for: dark mode, developer tools, system tray, dock.
"""

from PIL import Image, ImageDraw, ImageFont
import os
import math

# ─── Color Palette ───
BG_DARK       = (13, 15, 18, 255)       # #0d0f12 - main background
BG_SURFACE    = (21, 24, 32, 255)       # #151820 - card/surface
GREEN         = (52, 211, 153, 255)     # #34d399 - primary accent (emerald)
GREEN_DARK    = (34, 197, 134, 255)     # #22c55e - hover
GREEN_GLOW    = (52, 211, 153, 60)      # glow
WHITE         = (224, 224, 224, 255)    # #e0e0e0 - text
WHITE_DIM     = (160, 168, 184, 255)    # #a0a8b8 - subtle
BORDER        = (30, 33, 40, 255)       # #1e2128

SIZES = [16, 32, 48, 64, 128, 256, 512]

def draw_icon(draw: ImageDraw.Draw, size: int, cx: int, cy: int):
    """Draw the icon at given center and size."""
    r = size // 2 - 2  # radius with small padding
    
    # ─── Background circle with subtle gradient ───
    # Outer glow ring
    for i in range(3):
        alpha = 30 - i * 8
        glow_color = (*GREEN[:3], alpha)
        draw.ellipse(
            [cx - r - i, cy - r - i, cx + r + i, cy + r + i],
            outline=glow_color,
            width=2
        )
    
    # Main background circle
    draw.ellipse(
        [cx - r, cy - r, cx + r, cx + r],
        fill=BG_SURFACE,
        outline=BORDER,
        width=2
    )
    
    # Inner subtle highlight (top-left)
    highlight_r = r - 4
    draw.arc(
        [cx - highlight_r, cy - highlight_r, cx + highlight_r, cy + highlight_r],
        start=225, end=315,
        fill=(*WHITE[:3], 20),
        width=2
    )
    
    # ─── Central Symbol: Power Ring + "ON" ───
    symbol_r = r // 2
    
    if size >= 64:
        # Draw "ON" text for larger sizes
        font_size = max(12, size // 8)
        try:
            # Try to use a nice font
            font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", font_size)
        except:
            font = ImageFont.load_default()
        
        text = "ON"
        bbox = draw.textbbox((0, 0), text, font=font)
        tw = bbox[2] - bbox[0]
        th = bbox[3] - bbox[1]
        
        # Power ring behind text
        ring_r = max(symbol_r, tw // 2 + 8)
        draw.ellipse(
            [cx - ring_r, cy - ring_r, cx + ring_r, cy + ring_r],
            outline=GREEN,
            width=max(2, size // 32)
        )
        
        # Power gap at top
        gap_angle = 60
        draw.arc(
            [cx - ring_r, cy - ring_r, cx + ring_r, cy + ring_r],
            start=90 - gap_angle//2, end=90 + gap_angle//2,
            fill=BG_SURFACE,
            width=max(2, size // 32) + 2
        )
        
        # "ON" text
        draw.text(
            (cx - tw//2, cy - th//2 - 1),
            text,
            fill=GREEN,
            font=font
        )
    else:
        # Small sizes: just power symbol
        ring_r = symbol_r
        stroke = max(2, size // 16)
        
        # Full circle
        draw.ellipse(
            [cx - ring_r, cy - ring_r, cx + ring_r, cy + ring_r],
            outline=GREEN,
            width=stroke
        )
        # Power gap at top
        gap_angle = 70
        draw.arc(
            [cx - ring_r, cy - ring_r, cx + ring_r, cy + ring_r],
            start=90 - gap_angle//2, end=90 + gap_angle//2,
            fill=BG_SURFACE,
            width=stroke + 2
        )
        # Vertical line in gap
        line_len = ring_r // 2
        draw.line(
            [cx, cy - ring_r - 2, cx, cy - ring_r - 2 + line_len],
            fill=GREEN,
            width=stroke
        )

def generate_all():
    os.makedirs("/home/ahmed/workspace/microservices/odoo-noir/gui/build", exist_ok=True)
    
    for size in SIZES:
        img = Image.new("RGBA", (size, size), BG_DARK)
        draw = ImageDraw.Draw(img)
        cx = cy = size // 2
        
        draw_icon(draw, size, cx, cy)
        
        # Save individual size
        out_path = f"/home/ahmed/workspace/microservices/odoo-noir/gui/build/appicon_{size}.png"
        img.save(out_path, "PNG")
        print(f"Generated {out_path} ({size}x{size})")
    
    # Also save the 512 as the main appicon.png
    main_img = Image.new("RGBA", (512, 512), BG_DARK)
    draw = ImageDraw.Draw(main_img)
    draw_icon(draw, 512, 256, 256)
    main_img.save("/home/ahmed/workspace/microservices/odoo-noir/gui/build/appicon.png", "PNG")
    print("Generated main appicon.png (512x512)")

if __name__ == "__main__":
    generate_all()