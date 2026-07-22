"use client"

import * as React from "react"
import dayjs from "dayjs"
import customParseFormat from "dayjs/plugin/customParseFormat"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { cn } from "@/lib/utils"

dayjs.extend(customParseFormat)

export type DateTimePickerProps = {
  value: Date
  onChange: (date: Date) => void
  className?: string
}

const DATE_FORMAT = "YYYY-MM-DD"

/** 将日期与小时合并为本地时间的 Date（分秒毫秒归零） */
export function mergeDateAndHour(datePart: Date, hour: number): Date {
  const d = dayjs(datePart)
  return d.hour(hour).minute(0).second(0).millisecond(0).toDate()
}

function clampHour(h: number): number {
  if (!Number.isFinite(h)) return 0
  return Math.min(23, Math.max(0, Math.floor(h)))
}

function parseDateText(text: string): Date | null {
  const parsed = dayjs(text.trim(), DATE_FORMAT, true)
  if (!parsed.isValid()) return null
  return parsed.startOf("day").toDate()
}

function parseHourText(text: string): number | null {
  const trimmed = text.trim()
  if (trimmed === "") return null
  const n = Number.parseInt(trimmed, 10)
  if (!Number.isFinite(n) || n < 0 || n > 23) return null
  return n
}

/** 小时显示两位补零 */
function formatHourText(hour: number): string {
  return clampHour(hour).toString().padStart(2, "0")
}

/** 连体组内 Input：去掉独立边框/ring，高度交给外层 */
const segmentInputClass =
  "h-full rounded-none border-0 bg-transparent shadow-none focus-visible:border-transparent focus-visible:ring-0 dark:bg-transparent"

function DateTimePicker({ value, onChange, className }: DateTimePickerProps) {
  const valueDay = dayjs(value)
  const [dateOpen, setDateOpen] = React.useState(false)
  const [hourOpen, setHourOpen] = React.useState(false)
  const [dateText, setDateText] = React.useState(() => valueDay.format(DATE_FORMAT))
  const [hourText, setHourText] = React.useState(() => formatHourText(valueDay.hour()))

  React.useEffect(() => {
    const d = dayjs(value)
    setDateText(d.format(DATE_FORMAT))
    setHourText(formatHourText(d.hour()))
  }, [value])

  const emitChange = React.useCallback(
    (nextDatePart: Date, nextHour: number) => {
      onChange(mergeDateAndHour(nextDatePart, clampHour(nextHour)))
    },
    [onChange]
  )

  const commitDateText = React.useCallback(() => {
    const parsed = parseDateText(dateText)
    if (parsed) {
      const hour = parseHourText(hourText) ?? dayjs(value).hour()
      setDateText(dayjs(parsed).format(DATE_FORMAT))
      emitChange(parsed, hour)
    } else {
      setDateText(dayjs(value).format(DATE_FORMAT))
    }
  }, [dateText, hourText, value, emitChange])

  const commitHourText = React.useCallback(() => {
    const hour = parseHourText(hourText)
    if (hour !== null) {
      setHourText(formatHourText(hour))
      emitChange(value, hour)
    } else {
      setHourText(formatHourText(dayjs(value).hour()))
    }
  }, [hourText, value, emitChange])

  const calendarSelected = React.useMemo(() => {
    const parsed = dayjs(dateText, DATE_FORMAT, true)
    return parsed.isValid() ? parsed.toDate() : value
  }, [dateText, value])

  return (
    <div
      className={cn(
        "inline-flex h-9 shrink-0 items-stretch overflow-hidden rounded-md border border-input bg-transparent shadow-xs",
        "transition-[color,box-shadow] outline-none",
        "focus-within:border-ring focus-within:ring-ring/50 focus-within:ring-[3px]",
        "dark:bg-input/30",
        className,
        // 固定总宽：日期 7.5rem + 小时 2.25rem + H 1.75rem；放在 className 之后避免 w-full 拉满
        "w-[11.5rem]"
      )}
      data-slot="datetime-picker"
    >
      <Popover open={dateOpen} onOpenChange={setDateOpen} modal={false}>
        <PopoverAnchor asChild>
          <Input
            aria-label="Date"
            className={cn(
              segmentInputClass,
              "w-[7.5rem] px-2 font-mono tabular-nums text-center md:text-sm"
            )}
            value={dateText}
            onChange={(e) => setDateText(e.target.value)}
            onFocus={() => setDateOpen(true)}
            onBlur={() => {
              commitDateText()
              setDateOpen(false)
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                commitDateText()
                setDateOpen(false)
              }
            }}
          />
        </PopoverAnchor>
        <PopoverContent
          className="w-auto p-0"
          align="start"
          side="bottom"
          onOpenAutoFocus={(e) => e.preventDefault()}
          onInteractOutside={(e) => {
            const target = e.target as HTMLElement
            if (target.closest('[data-slot="datetime-picker"]')) {
              e.preventDefault()
            }
          }}
        >
          <div onMouseDown={(e) => e.preventDefault()}>
            <Calendar
              mode="single"
              selected={calendarSelected}
              onSelect={(d) => {
                if (!d) return
                const hour = parseHourText(hourText) ?? dayjs(value).hour()
                setDateText(dayjs(d).format(DATE_FORMAT))
                emitChange(d, hour)
                setDateOpen(false)
              }}
              defaultMonth={calendarSelected}
              classNames={{ today: "" }}
            />
          </div>
        </PopoverContent>
      </Popover>

      <Popover open={hourOpen} onOpenChange={setHourOpen} modal={false}>
        <PopoverAnchor asChild>
          <Input
            aria-label="Hour"
            className={cn(
              segmentInputClass,
              "w-9 border-l border-input px-0.5 font-mono tabular-nums text-center md:text-sm"
            )}
            inputMode="numeric"
            value={hourText}
            onChange={(e) => setHourText(e.target.value)}
            onFocus={() => setHourOpen(true)}
            onBlur={() => {
              commitHourText()
              setHourOpen(false)
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                commitHourText()
                setHourOpen(false)
              }
            }}
          />
        </PopoverAnchor>
        <PopoverContent
          className="w-auto p-1"
          align="start"
          side="bottom"
          onOpenAutoFocus={(e) => e.preventDefault()}
          onInteractOutside={(e) => {
            const target = e.target as HTMLElement
            if (target.closest('[data-slot="datetime-picker"]')) {
              e.preventDefault()
            }
          }}
        >
          <ul
            className="max-h-48 overflow-y-auto grid grid-cols-4 gap-0.5"
            role="listbox"
            aria-label="Hour"
          >
            {Array.from({ length: 24 }, (_, h) => (
              <li key={h} role="presentation">
                <button
                  type="button"
                  role="option"
                  aria-selected={dayjs(value).hour() === h}
                  className={cn(
                    "w-full rounded-sm px-2 py-1 text-sm font-mono tabular-nums hover:bg-accent hover:text-accent-foreground",
                    dayjs(value).hour() === h &&
                      "bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground"
                  )}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => {
                    setHourText(formatHourText(h))
                    emitChange(value, h)
                    setHourOpen(false)
                  }}
                >
                  {formatHourText(h)}
                </button>
              </li>
            ))}
          </ul>
        </PopoverContent>
      </Popover>

      {/* 小时单位标记：不可聚焦，固定文案 H */}
      <span
        aria-hidden="true"
        className="flex w-7 shrink-0 select-none items-center justify-center border-l border-input text-xs font-medium text-muted-foreground"
      >
        H
      </span>
    </div>
  )
}

export { DateTimePicker }
