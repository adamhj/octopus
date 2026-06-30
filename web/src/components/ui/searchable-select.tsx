'use client';

import React, { useState, useCallback, useMemo } from 'react';
import { X } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { cn } from '@/lib/utils';

/** onBlur 后延迟关闭弹窗的时间，确保点击下拉选项的 onClick 先触发 */
const BLUR_CLOSE_DELAY_MS = 150;

export interface SearchableSelectOption {
    value: string;
    label: string;
}

export interface SearchableSelectProps {
    value: string;
    onChange: (value: string) => void;
    options: SearchableSelectOption[];
    placeholder: string;
    className?: string;
    /** 无匹配结果时显示的文本 */
    noMatchText: string;
}

/**
 * 可搜索下拉框。
 * 文本框内容完全由本地 inputText 驱动；自定义输入跨聚焦保持。
 * onChange 仅在点击候选项或键盘回车时被调用；失焦/关闭弹窗不触发 onChange。
 * 支持键盘上下导航和回车选择。
 */
export function SearchableSelect({ value, onChange, options, placeholder, className, noMatchText }: SearchableSelectProps) {
    const [open, setOpen] = useState(false);
    const [inputText, setInputText] = useState(value);
    const [highlightIndex, setHighlightIndex] = useState(-1);

    const filtered = useMemo(() => {
        const s = inputText.toLowerCase();
        if (!s) return options;
        return options.filter(
            (o) => o.label.toLowerCase().includes(s) || o.value.toLowerCase().includes(s)
        );
    }, [options, inputText]);

    /** 选择候选项：调用 onChange 并同步 inputText 为候选项 label */
    const commitSelection = useCallback(
        (opt: SearchableSelectOption) => {
            onChange(opt.value);
            setInputText(opt.label);
            setOpen(false);
            setHighlightIndex(-1);
        },
        [onChange]
    );

    const handleFocus = useCallback(() => {
        setOpen(true);
        setHighlightIndex(-1);
    }, []);

    const handleClear = useCallback(
        (e: React.MouseEvent) => {
            e.preventDefault();
            e.stopPropagation();
            onChange('');
            setInputText('');
            setOpen(false);
            setHighlightIndex(-1);
        },
        [onChange]
    );

    const handleInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
        setInputText(e.target.value);
        setOpen(true);
        setHighlightIndex(-1);
    }, []);

    /** 键盘交互：上下导航、回车选择、Esc 关闭 */
    const handleKeyDown = useCallback(
        (e: React.KeyboardEvent) => {
            if (!open || filtered.length === 0) return;

            if (e.key === 'ArrowDown') {
                e.preventDefault();
                setHighlightIndex((prev) => (prev < filtered.length - 1 ? prev + 1 : 0));
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                setHighlightIndex((prev) => (prev > 0 ? prev - 1 : filtered.length - 1));
            } else if (e.key === 'Enter') {
                if (highlightIndex >= 0 && highlightIndex < filtered.length) {
                    e.preventDefault();
                    commitSelection(filtered[highlightIndex]);
                }
            } else if (e.key === 'Escape') {
                setOpen(false);
                setHighlightIndex(-1);
            }
        },
        [open, filtered, highlightIndex, commitSelection]
    );

    return (
        <Popover open={open && options.length > 0} onOpenChange={setOpen} modal={false}>
            <div className="relative">
                <PopoverAnchor asChild>
                    <Input
                        role="combobox"
                        aria-expanded={open}
                        placeholder={placeholder}
                        value={inputText}
                        onChange={handleInputChange}
                        onFocus={handleFocus}
                        onBlur={() => {
                            setTimeout(() => {
                                setOpen(false);
                                setHighlightIndex(-1);
                            }, BLUR_CLOSE_DELAY_MS);
                        }}
                        onKeyDown={handleKeyDown}
                        className={cn('w-full pr-8', className)}
                    />
                </PopoverAnchor>
                {value && (
                    <button
                        type="button"
                        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                        onMouseDown={(e) => e.preventDefault()}
                        onClick={handleClear}
                        tabIndex={-1}
                    >
                        <X className="h-4 w-4" />
                    </button>
                )}
            </div>
            <PopoverContent
                className="p-1"
                style={{
                    width: 'fit-content',
                    minWidth: 'var(--radix-popover-trigger-width)',
                    maxWidth: 'calc(100vw - 2rem)',
                } as React.CSSProperties}
                align="start"
                side="bottom"
                onOpenAutoFocus={(e) => e.preventDefault()}
            >
                <div className="flex flex-col max-h-48 overflow-y-auto">
                    {filtered.map((opt, idx) => (
                        <button
                            key={opt.value}
                            type="button"
                            className={cn(
                                'rounded-sm px-2 py-1.5 text-sm text-left whitespace-nowrap',
                                (value === opt.value || idx === highlightIndex)
                                    ? 'bg-accent text-accent-foreground'
                                    : 'hover:bg-accent hover:text-accent-foreground'
                            )}
                            onMouseDown={(e) => e.preventDefault()}
                            onClick={() => commitSelection(opt)}
                            onMouseEnter={() => setHighlightIndex(idx)}
                        >
                            {opt.label}
                        </button>
                    ))}
                    {filtered.length === 0 && (
                        <div className="px-2 py-1.5 text-sm text-muted-foreground">{noMatchText}</div>
                    )}
                </div>
            </PopoverContent>
        </Popover>
    );
}
