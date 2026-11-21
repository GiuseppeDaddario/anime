-- AnimeWatcher: a liquid-glass "Skip intro" button layered over the user's normal mpv.
-- It shows during the opening AniSkip (api.aniskip.com) knows for this episode, and a
-- click jumps to the end of it.
local mp = require "mp"
local msg = require "mp.msg"
local options = require "mp.options"
local utils = require "mp.utils"

local opts = {
    mal_id = 0,         -- MyAnimeList id and episode number, passed by `watch`
    episode = 0,
    intro_start = -1,   -- An intro, in seconds, to use instead of asking AniSkip
    intro_end = -1,
    prompt = 8,         -- Seconds the button stays up once the intro starts
    hide_after = 4,
    always_visible = false,
    refraction = true,
    font = "SF Pro Text",
}
options.read_options(opts, "animewatcher")
opts.prompt = math.max(0, opts.prompt)
opts.hide_after = math.max(0.5, opts.hide_after)

local API = "https://api.aniskip.com/v2"
local LABEL = "Skip intro"
local END_GUARD = 2   -- No button for the last seconds of an intro: nothing is left to skip.
local SETTLE = 0.25   -- Seconds the button must keep its place before the glass is redrawn.

-- Glisse's dark glass (github.com/0kzh/glisse), in its CSS px: a 40 px pill with a 14 px
-- label after a 20 px icon, tinted --lg-glass and lit by the rim refraction.ts paints.
local GLASS = {
    height = 40, pad_left = 12, pad_right = 16, icon = 20, gap = 8, font = 14,
    tint = {40, 40, 40}, active = {70, 70, 70}, opacity = 0.62,
    edge = 0.08, edge_width = 0.5, shine = 0.13, rim_width = 1.22, glow = 0.035, grow = 6,
}

local overlay = mp.create_osd_overlay("ass-events")
overlay.z = 1100
local section = "animewatcher-skip-button"
local state = {
    active = false, paused = false, inside = false, dismissed = false,
    hovered = false, pressed = false, bindings = false,
    opacity = 0, hover = 0, glow = 0, grow = 1, grow_velocity = 0,
    last_activity = -100, prompt_until = -100, last_tick = mp.get_time(),
    mouse = {x = -1, y = -1, hover = false}, lookup = 0,
}
local glass = {files = {}, changed = -100}
local tick, wake
local timer, hide_timer, prompt_timer

local function clamp(value, low, high)
    return math.max(low, math.min(high, value))
end

-- ASS drawing of a rounded rectangle, clockwise; `reverse` traces it the other way to
-- cut it out of a shape drawn around it.
local function rounded(x, y, w, h, r, reverse)
    local k = r * 0.55228475
    local start = {x+r, y}
    local path = {
        {x+w-r, y}, {x+w-r+k, y, x+w, y+r-k, x+w, y+r},
        {x+w, y+h-r}, {x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h},
        {x+r, y+h}, {x+r-k, y+h, x, y+h-r+k, x, y+h-r},
        {x, y+r}, {x, y+r-k, x+r-k, y, x+r, y},
    }
    if reverse then
        local back = {}
        for i = #path, 1, -1 do
            local from = path[i-1] or start
            local to = {from[#from-1], from[#from]}
            local segment = path[i]
            back[#back+1] = #segment == 2 and to or {segment[3], segment[4], segment[1], segment[2], to[1], to[2]}
        end
        path = back
    end
    local out = {string.format("m %.2f %.2f", start[1], start[2])}
    for _, segment in ipairs(path) do
        local numbers = {}
        for i, value in ipairs(segment) do numbers[i] = string.format("%.2f", value) end
        out[#out+1] = (#segment == 2 and "l " or "b ") .. table.concat(numbers, " ")
    end
    return table.concat(out, " ")
end

local function polygon(points)
    local out = {}
    for i, point in ipairs(points) do out[i] = string.format("%.2f %.2f", point[1], point[2]) end
    return "m " .. out[1] .. " l " .. table.concat(out, " ", 2)
end

-- A ring's sector between two depths of a circle, from one angle to another (degrees,
-- clockwise from the right, as the screen's y points down).
local function sector(cx, cy, outer, inner, from, to)
    local points, steps = {}, math.max(1, math.ceil((to - from) / 5))
    for i = 0, steps do
        local a = math.rad(from + (to - from) * i / steps)
        points[#points+1] = {cx + outer * math.cos(a), cy + outer * math.sin(a)}
    end
    for i = steps, 0, -1 do
        local a = math.rad(from + (to - from) * i / steps)
        points[#points+1] = {cx + inner * math.cos(a), cy + inner * math.sin(a)}
    end
    return polygon(points)
end

-- libass sizes a font by its ascender and descender, about 1.19 of the em CSS sizes it by.
local ASS_EM = 1.19

local measured = {}
-- The label's width at a font size, in OSD px. libass measures it at that very size:
-- hinting makes small text a few percent wider than large text scaled down.
local function label_width(size, w, h)
    local key = string.format("%s %.1f %d %d", opts.font, size, w, h)
    if not measured[key] then
        local probe = mp.create_osd_overlay("ass-events")
        probe.res_x, probe.res_y, probe.hidden, probe.compute_bounds = w, h, true, true
        probe.data = string.format("{\\an7\\pos(0,0)\\fn%s\\fs%.2f\\b500\\bord0\\shad0\\blur0}%s",
            (opts.font:gsub("[\\{}]", "")), size, LABEL)
        local bounds = probe:update()
        probe:remove()
        local width = bounds and bounds.x1 and bounds.x0 and bounds.x1 - bounds.x0
        measured[key] = width and width > 0 and width or 0.38 * size * #LABEL
    end
    return measured[key]
end

local function geometry()
    local w, h = mp.get_osd_size()
    if not w or not h or w < 240 or h < 160 then return nil end
    local margins = mp.get_property_native("osd-dimensions", {})
    local ml, mr, mt, mb = margins.ml or 0, margins.mr or 0, margins.mt or 0, margins.mb or 0
    local left, right, top, bottom = math.max(0, ml), math.max(0, mr), math.max(0, mt), math.max(0, mb)
    local hidpi = mp.get_property_number("display-hidpi-scale", 1) or 1
    -- Device px per glisse px: the pill is about 6% of the window's height in points.
    local scale = clamp(0.058 * h / hidpi, 34, 64) * hidpi / GLASS.height
    local function length(s)
        return GLASS.pad_left + GLASS.icon + GLASS.gap + GLASS.pad_right
            + label_width(GLASS.font * ASS_EM * s, w, h) / s
    end
    local css_width = length(scale)
    if (w - left - right) / (css_width + 54) < scale then
        scale = (w - left - right) / (css_width + 54)
        css_width = length(scale)
    end
    local width, height = css_width * scale, GLASS.height * scale
    local x = math.max(left + 16*scale, w - right - 27*scale - width)
    local y = math.max(top + 16*scale, h - bottom - 83*scale - height)
    return {x=x, y=y, w=width, h=height, scale=scale, css_width=css_width, hidpi=hidpi,
        screen_w=w, screen_h=h, video={x=ml, y=mt, w=w-ml-mr, h=h-mt-mb}}
end

local function hit()
    local r, mouse = state.rect, state.mouse
    return r and mouse.hover ~= false and mouse.x >= r.x and mouse.x <= r.x+r.w
        and mouse.y >= r.y and mouse.y <= r.y+r.h
end

local function input_area(enabled)
    if enabled and state.rect then
        local r = state.rect
        mp.set_mouse_area(r.x, r.y, r.x+r.w, r.y+r.h, section)
        if not state.bindings then
            mp.enable_key_bindings(section)
            state.bindings = true
        end
    elseif state.bindings then
        mp.disable_key_bindings(section)
        mp.set_mouse_area(0, 0, 0, 0, section)
        state.bindings = false
        state.pressed = false
    end
end

-- The glass itself, refracting the video, is a GPU shader on mpv's OUTPUT stage: glisse's
-- lens (lens.ts lensMap with refraction.ts GLASS) behind a 3 px blur and saturate(1.5),
-- as its backdrop-filter does. mpv's own renderer takes no live shader parameters, so
-- each placement of the button gets a shader of its own, swapped in while it is shown.
-- Lengths are glisse's CSS px; the pill is 40 high, so its rim bends 10 px deep, by up
-- to 0.45 of 40, and red bends 8% further than blue.
local SHADER = [[
//!HOOK OUTPUT
//!BIND HOOKED
//!DESC AnimeWatcher skip intro glass

const vec2 aw_center = vec2($cx, $cy);
const vec2 aw_half = vec2($hw, $hh);
const vec2 aw_expect = vec2($ew, $eh);
const float aw_hidpi = $hidpi;
const float aw_length = $length;
const vec2 aw_across = vec2($across_radius, $across_scale);
const vec2 aw_down = vec2($down_radius, $down_scale);

float aw_box(vec2 p, vec2 half_size, float radius) {
    vec2 q = abs(p) - half_size + radius;
    return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - radius;
}

float aw_slope(float x, vec2 arc) {
    float inside = min(x, 0.999 * arc.x);
    return inside / sqrt(arc.x * arc.x - inside * inside) * arc.y;
}

vec3 aw_blur(vec2 pos, float sigma) {
    vec2 reach = HOOKED_pt * sigma * 2.5;
    vec3 sum = HOOKED_tex(pos).rgb;
    float total = 1.0;
    for (int i = 0; i < 16; i++) {
        float r = sqrt((float(i) + 0.5) / 16.0);
        float a = float(i) * 2.39996323;
        float weight = exp(-3.125 * r * r);
        sum += weight * HOOKED_tex(pos + reach * r * vec2(cos(a), sin(a))).rgb;
        total += weight;
    }
    return sum / total;
}

vec4 hook() {
    vec4 color = HOOKED_tex(HOOKED_pos);
    vec2 size = HOOKED_size;
    // Drawn only at the window's size: a video screenshot has no label over it.
    if (any(greaterThan(abs(size - aw_expect), vec2(2.0)))
        && any(greaterThan(abs(size - aw_expect * aw_hidpi), vec2(2.0))))
        return color;
    float css = aw_half.y * size.y / 20.0;
    vec2 p = (HOOKED_pos - aw_center) * size / css;
    float cover = clamp(0.5 - aw_box(p, vec2(aw_length, 20.0), 20.0) * css, 0.0, 1.0);
    if (cover <= 0.0)
        return color;
    float inner = aw_box(p, vec2(max(aw_length - 10.0, 0.0), 10.0), 10.0) * 0.0707107;
    float falloff = 1.0 - 1.0 / (exp(3.5449077 * inner) + 1.0);
    vec2 pull = clamp(sign(p) * vec2(aw_slope(abs(p.x), aw_across), aw_slope(abs(p.y), aw_down))
        * falloff, -1.0, 1.0);
    vec2 shift = -9.0 * pull * css / size;
    float sigma = 3.0 * css;
    vec3 glass = vec3(aw_blur(HOOKED_pos + shift * 1.08, sigma).r,
                      aw_blur(HOOKED_pos + shift * 1.04, sigma).g,
                      aw_blur(HOOKED_pos + shift, sigma).b);
    glass = mix(vec3(dot(glass, vec3(0.213, 0.715, 0.072))), glass, 1.5);
    color.rgb = mix(color.rgb, clamp(glass, 0.0, 1.0), cover);
    return color;
}
]]

-- The circular arc that sags by `sag` across `half`, scaled so its slope averages a
-- half over that span (glisse's lens.ts arc()).
local function arc(sag, half)
    local radius = (half * half + sag * sag) / (2 * sag)
    local sum = 0
    for i = 0, 200 do
        local x = i / 200 * half
        local slope = x / math.sqrt(radius * radius - x * x)
        sum = sum + ((i == 0 or i == 200) and slope / 2 or slope)
    end
    return radius, sum > 0 and 0.5 * 200 / sum or 1
end

-- The shader is placed relative to the video, which zoom and pan move under a still button.
local function placement(r)
    return r and string.format("%.1f %.1f %.1f %.1f %.1f %.1f %.1f %.1f",
        r.x, r.y, r.w, r.h, r.video.x, r.video.y, r.video.w, r.video.h)
end

local function shader_for(r)
    local key = placement(r)
    if glass.files[key] then return glass.files[key] end
    local length = r.css_width / 2
    local across_radius, across_scale = arc(19, length)
    local down_radius, down_scale = arc(19, 20)
    local values = {
        cx = (r.x + r.w/2 - r.video.x) / r.video.w, cy = (r.y + r.h/2 - r.video.y) / r.video.h,
        hw = r.w/2 / r.video.w, hh = r.h/2 / r.video.h, ew = r.video.w, eh = r.video.h,
        hidpi = r.hidpi, length = length, across_radius = across_radius,
        across_scale = across_scale, down_radius = down_radius, down_scale = down_scale,
    }
    local code = SHADER:gsub("%$([%w_]+)", function(name) return string.format("%.6f", values[name]) end)
    local made, path = pcall(os.tmpname)
    local file = made and io.open(path, "w")
    if not file then return nil end
    file:write(code)
    file:close()
    glass.files[key] = path
    return path
end

-- Swaps the shader in while the button is up and holds still, and out otherwise.
local function update_glass(now, shown)
    local r = state.rect
    local key = placement(r)
    if key ~= glass.key then
        if glass.key and key then
            glass.changed = now
            if glass.timer then glass.timer:kill() end
            glass.timer = mp.add_timeout(SETTLE + 0.02, wake)
        end
        glass.key = key
    end
    local path = opts.refraction and shown and r and now - glass.changed >= SETTLE and shader_for(r) or nil
    if path ~= glass.path then
        if glass.path then mp.commandv("change-list", "glsl-shaders", "remove", glass.path) end
        if path then mp.commandv("change-list", "glsl-shaders", "append", path) end
        glass.path = path
    end
end

local function bgr(rgb)
    return string.format("%02X%02X%02X", rgb[3], rgb[2], rgb[1])
end

local function alpha(opacity)
    return string.format("%02X", math.floor(255 * (1-clamp(opacity*state.opacity, 0, 1)) + 0.5))
end

local function publish(r)
    local intro = state.intro
    mp.set_property_native("user-data/animewatcher/skip-button", {
        x=r and r.x, y=r and r.y, width=r and r.w, height=r and r.h,
        visible=r ~= nil and state.opacity > 0.12, hovered=state.hovered,
        intro_start=intro and intro.start, intro_end=intro and intro.stop,
    })
end

local function render()
    local r = state.rect
    if not r or state.opacity < 0.005 then
        overlay:remove()
        input_area(false)
        publish(nil)
        return
    end
    local s = r.scale
    local cx, cy = r.x + r.w/2, r.y + r.h/2
    local hw, hh = r.w/2 * state.grow, r.h/2 * state.grow
    local x, y, w, h = cx - hw, cy - hh, hw*2, hh*2
    local events = {}
    local function shape(drawing, color, opacity)
        events[#events+1] = "{\\rDefault\\an7\\pos(0,0)\\p1\\bord0\\shad0\\blur0\\1c&H" .. color
            .. "&\\1a&H" .. alpha(opacity) .. "&}" .. drawing .. "{\\p0}"
    end

    -- Tint, lighter while hovered or pressed, and the press's faint glow.
    local lit, tint = math.max(state.hover, state.glow), {}
    for i = 1, 3 do tint[i] = math.floor(GLASS.tint[i] + (GLASS.active[i]-GLASS.tint[i]) * lit + 0.5) end
    local pill = rounded(x, y, w, h, hh)
    shape(pill, bgr(tint), GLASS.opacity)
    if state.glow > 0.004 then shape(pill, "FFFFFF", GLASS.glow * state.glow) end

    -- The 0.5 px edge line, then the rim's light over it (refraction.ts rimMap): brightest
    -- at the very edge and gone 1.22 px in, lit from the top left, so along the top and
    -- left and, reflected, the bottom and right. It fades out over the other two corners
    -- and widens on these two.
    local edge = GLASS.edge_width * s
    shape(pill .. " " .. rounded(x+edge, y+edge, w-2*edge, h-2*edge, hh-edge, true), "FFFFFF", GLASS.edge)
    local bands, width = 3, GLASS.rim_width * s
    local function band(j)  -- 1 - t^1.5 averaged over the band's share t of the rim
        local a, b = (j-1) / bands, j / bands
        return 1 - (b^2.5 - a^2.5) / (2.5 * (b-a))
    end
    local left, right = cx - hw + hh, cx + hw - hh
    for j = 1, bands do
        local d0, d1 = width * (j-1) / bands, width * j / bands
        shape(polygon({{left, y+d0}, {right, y+d0}, {right, y+d1}, {left, y+d1}}), "FFFFFF", GLASS.shine * band(j))
        shape(polygon({{left, y+h-d1}, {right, y+h-d1}, {right, y+h-d0}, {left, y+h-d0}}), "FFFFFF", GLASS.shine * band(j))
    end
    for _, cap in ipairs({{right, -90}, {left, 90}}) do
        for from = cap[2], cap[2] + 170, 10 do
            local mid = math.rad(from + 5)
            local facing = math.abs(math.cos(mid) + math.sin(mid))
            local light = GLASS.shine * math.min(1, facing)^1.5
            local wide = width * math.sqrt(math.max(1, facing))
            for j = 1, light > 0.002 and bands or 0 do
                shape(sector(cap[1], cy, hh - wide*(j-1)/bands, hh - wide*j/bands, from, from + 10),
                    "FFFFFF", light * band(j))
            end
        end
    end

    -- A forward-to-end icon, then the label.
    local u = s * state.grow
    local ix = x + GLASS.pad_left * u
    shape(polygon({{ix+2.5*u, cy-5*u}, {ix+9.5*u, cy}, {ix+2.5*u, cy+5*u}}) .. " "
        .. polygon({{ix+9.5*u, cy-5*u}, {ix+16.5*u, cy}, {ix+9.5*u, cy+5*u}}) .. " "
        .. rounded(ix+16.5*u, cy-5*u, 2*u, 10*u, u), "FFFFFF", 1)
    events[#events+1] = string.format(
        "{\\rDefault\\an4\\pos(%.2f,%.2f)\\fn%s\\fs%.2f\\b500\\bord0\\shad0\\blur0\\1c&HFFFFFF&\\1a&H%s&}%s",
        ix + (GLASS.icon + GLASS.gap) * u, cy - 0.4*u, (opts.font:gsub("[\\{}]", "")),
        GLASS.font * ASS_EM * u, alpha(1), LABEL)

    local data = table.concat(events, "\n")
    if data ~= overlay.data or overlay.res_x ~= r.screen_w or overlay.res_y ~= r.screen_h then
        overlay.res_x, overlay.res_y, overlay.data = r.screen_w, r.screen_h, data
        overlay:update()
    end
    input_area(state.active and state.opacity > 0.12 and not state.dismissed)
    publish(r)
end

-- Glisse's spring (spring.ts): critically, over- or under-damped, solved exactly.
local function spring(value, velocity, target, stiffness, damping, dt)
    local x, v, half = value - target, velocity, damping / 2
    local discriminant = half*half - stiffness
    if math.abs(discriminant) < 1e-4 then
        local decay, b = math.exp(-half*dt), v + half*x
        return target + decay*(x + b*dt), decay*(v - half*b*dt)
    elseif discriminant > 0 then
        local root = math.sqrt(discriminant)
        local r1, r2 = -half + root, -half - root
        local a = (v - r2*x) / (r1 - r2)
        local b = x - a
        return target + a*math.exp(r1*dt) + b*math.exp(r2*dt), a*r1*math.exp(r1*dt) + b*r2*math.exp(r2*dt)
    end
    local omega, decay = math.sqrt(-discriminant), math.exp(-half*dt)
    local cos, sin = math.cos(omega*dt), math.sin(omega*dt)
    return target + decay*(x*cos + (v + half*x)/omega*sin), decay*(v*cos - (half*v + stiffness*x)/omega*sin)
end

tick = function()
    local now = mp.get_time()
    local dt = clamp(now-state.last_tick, 0.001, 0.1)
    state.last_tick = now
    state.rect = geometry()
    state.hovered = hit() and true or false
    local visible = state.active and state.inside and not state.dismissed and state.rect ~= nil
        and (opts.always_visible or state.paused or state.hovered or now < state.prompt_until
        or now-state.last_activity < opts.hide_after)
    local opacity_target, hover_target = visible and 1 or 0, state.hovered and 1 or 0
    -- Pressing lights the glass and grows it at once, as glisse's pointer press does;
    -- both ease back on release.
    local grow_target = 1
    if state.pressed and state.rect then
        state.glow = 1
        grow_target = 1 + GLASS.grow / state.rect.css_width
    end
    local stiffness, damping = 230, 23.7
    if state.pressed then stiffness, damping = 417.6, 29.2 end
    state.grow, state.grow_velocity = spring(state.grow, state.grow_velocity, grow_target, stiffness, damping, dt)
    state.opacity = state.opacity + (opacity_target-state.opacity) * (1-math.exp(-dt/0.06))
    state.hover = state.hover + (hover_target-state.hover) * (1-math.exp(-dt/0.07))
    state.glow = state.glow * math.exp(-dt/0.075)
    if state.glow < 0.004 then state.glow = 0 end
    update_glass(now, visible)
    render()
    if math.abs(state.opacity-opacity_target) < 0.003 and math.abs(state.hover-hover_target) < 0.003
        and state.glow == 0 and math.abs(state.grow-grow_target) < 1e-4 and math.abs(state.grow_velocity) < 1e-3 then
        state.opacity, state.hover, state.grow, state.grow_velocity = opacity_target, hover_target, grow_target, 0
        render()
        timer:kill()
    end
end

timer = mp.add_periodic_timer(1/60, function() tick() end)
timer:kill()
wake = function()
    timer:resume()
    tick()
end

local function show()
    state.last_activity = mp.get_time()
    if hide_timer then hide_timer:kill() end
    hide_timer = mp.add_timeout(opts.hide_after+0.05, wake)
    wake()
end

local function update_position(position)
    local intro = state.intro
    local inside = intro ~= nil and position ~= nil and position >= intro.start
        and position < math.max(intro.start + 0.5, intro.stop - END_GUARD)
    if inside == state.inside then return end
    -- Leaving the intro re-arms the button, so seeking back into it offers the skip again.
    state.inside, state.dismissed = inside, false
    if inside then
        state.prompt_until = mp.get_time() + opts.prompt
        if prompt_timer then prompt_timer:kill() end
        prompt_timer = mp.add_timeout(opts.prompt+0.05, wake)
        show()
    else
        wake()
    end
end

local function skip()
    local intro = state.intro
    if state.dismissed or not state.inside or not state.active or not intro
        or not mp.get_property_bool("seekable", false) then return end
    if not mp.commandv("seek", string.format("%.3f", intro.stop), "absolute+exact") then return end
    state.dismissed = true
    state.pressed = false
    state.opacity = 0
    if hide_timer then hide_timer:kill() end
    wake()
end

local function click()
    if state.opacity > 0.12 and hit() then skip() end
end

-- AniSkip --------------------------------------------------------------------------

local function request(url, callback)
    mp.command_native_async({
        name = "subprocess", playback_only = false, capture_stdout = true, capture_stderr = true,
        args = {"curl", "--silent", "--show-error", "--globoff", "--max-time", "10",
            "--user-agent", "AnimeWatcher", url},
    }, function(success, result)
        local data = success and result and result.status == 0 and utils.parse_json(result.stdout or "")
        callback(type(data) == "table" and data or nil)
    end)
end

-- AniSkip's relation rules move an episode numbered on from an earlier season to the
-- season it belongs to (Attack on Titan episode 30 is season 2 episode 5).
local function redirect(rules, anime, episode)
    for _, rule in ipairs(rules and rules.found and rules.rules or {}) do
        local from, to = rule.from or {}, rule.to or {}
        if from.start and to.malId and to.start and episode >= from.start
            and (not from["end"] or episode <= from["end"]) then
            return to.malId, to.start + episode - from.start
        end
    end
    return anime, episode
end

-- The best opening AniSkip returned: a plain one before one mixed into the episode,
-- and only a span that fits in this video.
local function pick(data, duration)
    local best
    for _, result in ipairs(data and data.found and data.results or {}) do
        local interval = result.interval or {}
        local start, stop = tonumber(interval.startTime), tonumber(interval.endTime)
        if start and stop and start >= 0 and stop - start >= 5 and stop - start <= 360
            and (duration <= 0 or stop <= duration + 1)
            and (not best or (best.kind ~= "op" and result.skipType == "op")) then
            best = {start = start, stop = stop, kind = result.skipType}
        end
    end
    return best
end

local function set_intro(intro, source)
    state.intro = intro
    if intro then
        msg.info(string.format("Intro %d:%02d-%d:%02d (%s)", math.floor(intro.start/60), math.floor(intro.start%60),
            math.floor(intro.stop/60), math.floor(intro.stop%60), source))
    end
    state.inside = false
    update_position(mp.get_property_number("time-pos"))
    wake()
end

local function find_intro()
    state.lookup = state.lookup + 1
    local lookup = state.lookup
    if opts.intro_start >= 0 and opts.intro_end > opts.intro_start then
        return set_intro({start = opts.intro_start, stop = opts.intro_end, kind = "op"}, "script-opts")
    end
    if opts.mal_id <= 0 or opts.episode <= 0 then return end
    local current = function() return lookup == state.lookup end
    request(string.format("%s/relation-rules/%d", API, math.floor(opts.mal_id)), function(rules)
        if not current() then return end
        local anime, episode = redirect(rules, math.floor(opts.mal_id), opts.episode)
        local duration = mp.get_property_number("duration", 0) or 0
        local url = string.format("%s/skip-times/%d/%s?types[]=op&types[]=mixed-op&episodeLength=",
            API, math.floor(anime), string.format("%.10g", episode))
        -- Skip times measured on a video of this length first, then on any length.
        request(url .. (duration > 0 and string.format("%.3f", duration) or "0"), function(matched)
            if not current() then return end
            local intro = pick(matched, duration)
            if intro or duration <= 0 then
                if not intro then msg.info("AniSkip has no intro for this episode") end
                return set_intro(intro, "AniSkip")
            end
            request(url .. "0", function(any)
                if not current() then return end
                local found = pick(any, duration)
                if not found then msg.info("AniSkip has no intro for this episode") end
                set_intro(found, "AniSkip, from another release")
            end)
        end)
    end)
end

mp.set_key_bindings({
    {"MBTN_LEFT", click,
        function() state.pressed = hit() and true or false; wake() end,
        function() local pressed = state.pressed; state.pressed = false; if pressed then click() end; wake() end},
    {"MBTN_LEFT_DBL", "ignore"},
}, section, "force")
mp.disable_key_bindings(section)

mp.observe_property("mouse-pos", "native", function(_, mouse)
    if not mouse then return end
    local moved = mouse.x ~= state.mouse.x or mouse.y ~= state.mouse.y
    state.mouse = mouse
    if mouse.hover == false then state.pressed = false; wake()
    elseif moved then show()
    else wake() end
end)
mp.observe_property("time-pos", "number", function(_, position) update_position(position) end)
mp.observe_property("osd-dimensions", "native", function() wake() end)
mp.observe_property("pause", "bool", function(_, paused) state.paused = paused; wake() end)
mp.observe_property("video-params", "native", function(_, params)
    state.active = params ~= nil and (params.w or 0) > 0
    wake()
end)
mp.register_event("start-file", function()
    state.lookup = state.lookup + 1
    state.intro, state.inside, state.dismissed = nil, false, false
    state.active, state.pressed, state.opacity = false, false, 0
    wake()
end)
mp.register_event("file-loaded", function()
    local params = mp.get_property_native("video-params")
    state.active = params ~= nil and (params.w or 0) > 0
    find_intro()
    wake()
end)
mp.register_event("end-file", function() state.active = false; state.pressed = false; wake() end)
mp.register_event("shutdown", function()
    timer:kill()
    for _, pending in pairs({hide_timer, prompt_timer, glass.timer}) do pending:kill() end
    input_area(false)
    overlay:remove()
    if glass.path then mp.commandv("change-list", "glsl-shaders", "remove", glass.path) end
    for _, path in pairs(glass.files) do os.remove(path) end
    mp.set_property_native("user-data/animewatcher/skip-button", nil)
end)
mp.register_script_message("show", show)
mp.register_script_message("skip", skip)
