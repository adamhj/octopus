'use client';

import { useState, useCallback, useMemo, useEffect, Fragment } from 'react';
import dayjs from 'dayjs';
import {
    BarChart,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
    ResponsiveContainer,
} from 'recharts';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';
import { PageWrapper } from '@/components/common/PageWrapper';
import { DateTimePicker } from '@/components/ui/datetime-picker';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import { SearchableSelect } from '@/components/ui/searchable-select';
import { useStatsDetailChart } from '@/api/endpoints/stats';
import { useStatsQueryStore, type StatsPeriod } from './store';
import { useChannelOptions } from '@/api/endpoints/channel';
import { useModelOptions } from '@/api/endpoints/model';
import { logger } from '@/lib/logger';

// ==================== 系列色板（低彩度分类色） ====================

/**
 * 跨色相、中低彩度分类色。
 * 主题 chart-1..5 是同一绿相上的明度阶梯，多系列几乎不可辨，故不用其循环。
 * 首色仍贴近主题绿（hue≈145），其余在色环上拉开，chroma 控制在「不刺眼」区间。
 */
const SERIES_PALETTE = [
    'oklch(0.62 0.10 145)', // 绿（贴 primary）
    'oklch(0.58 0.09 250)', // 蓝
    'oklch(0.62 0.09 50)',  // 暖琥珀
    'oklch(0.58 0.09 310)', // 紫
    'oklch(0.60 0.08 200)', // 青
    'oklch(0.58 0.09 25)',  // 赭红
    'oklch(0.56 0.08 170)', // 青绿
    'oklch(0.60 0.08 280)', // 靛
] as const;

/** 缓存命中块透明度（低彩度下略提高，避免与背景糊在一起） */
const CACHE_HIT_OPACITY = 0.7;

/**
 * 按系列序号取色：先走跨色相色板；
 * 超出后用 color-mix 与 muted 混合，形成低艳度第二圈。
 */
function seriesFill(index: number): string {
    const base = SERIES_PALETTE[index % SERIES_PALETTE.length];
    if (index < SERIES_PALETTE.length) return base;
    return `color-mix(in oklch, ${base} 65%, var(--muted) 35%)`;
}

// ==================== 工具函数 ====================

function formatNumber(n: number): string {
    return n.toLocaleString('en-US');
}

function formatCost(n: number): string {
    return n.toFixed(4);
}

function comboKey(channelId: number, modelName: string): string {
    return `c${channelId}_${modelName.replace(/[^a-zA-Z0-9_-]/g, '_')}`;
}

function formatTimeLabel(time: string, period: StatsPeriod): string {
    switch (period) {
        case 'hour': {
            const m = time.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2})$/);
            if (m) return `${m[2]}-${m[3]} ${m[4]}:00`;
            return time;
        }
        case 'day': {
            const m = time.match(/^(\d{4})-(\d{2})-(\d{2})$/);
            if (m) return `${m[2]}-${m[3]}`;
            return time;
        }
        case 'week': {
            const start = dayjs(time);
            const end = start.add(6, 'day');
            return `${start.format('MM/DD')}-${end.format('MM/DD')}`;
        }
        case 'month': {
            const m = time.match(/^(\d{4})-(\d{2})$/);
            if (m) return `${m[1]}-${m[2]}`;
            return time;
        }
    }
}

/**
 * 根据数据点数量计算 X 轴间隔，自动稀疏标签避免重叠。
 * 数据点少时显示所有标签，数据点多时每隔若干个标签显示一个。
 */
function computeXAxisInterval(totalBuckets: number): number {
    if (totalBuckets <= 12) return 0; // 显示全部标签
    if (totalBuckets <= 48) return Math.ceil(totalBuckets / 8);
    return Math.ceil(totalBuckets / 6);
}

// ==================== 组合信息类型 ====================

interface ComboInfo {
    key: string;
    channelName: string;
    modelName: string;
}

// ==================== Hover 状态类型 ====================

interface HoverInfo {
    comboKey: string;
    bucketIndex: number;
    /** 鼠标触发时的视口 X 坐标，用于 fixed 定位 */
    clientX: number;
    clientY: number;
}

// ==================== 色块级悬浮 Tooltip ====================

/** Tooltip 估计宽度，用于右边界翻转判断 */
const ESTIMATED_TOOLTIP_WIDTH = 240;
/** Tooltip 估计高度 */
const ESTIMATED_TOOLTIP_HEIGHT = 150;

function ComboTooltipOverlay({ hoverInfo, rechartsData, comboColors, t }: {
    hoverInfo: HoverInfo | null;
    rechartsData: Record<string, number | string>[];
    comboColors: Array<ComboInfo & { fill: string }>;
    t: (key: string) => string;
}) {
    if (!hoverInfo) return null;

    const combo = comboColors.find(c => c.key === hoverInfo.comboKey);
    if (!combo) return null;

    const bucketData = rechartsData[hoverInfo.bucketIndex];
    if (!bucketData) return null;

    const missTokens = (bucketData[`${combo.key}-miss`] as number) || 0;
    const hitTokens = (bucketData[`${combo.key}-hit`] as number) || 0;
    const totalTokens = missTokens + hitTokens;
    if (totalTokens === 0) return null;

    const cacheHitRate = (hitTokens / totalTokens) * 100;
    const inputTokens = (bucketData[`${combo.key}-input`] as number) || 0;
    const outputTokens = (bucketData[`${combo.key}-output`] as number) || 0;
    const callCount = (bucketData[`${combo.key}-calls`] as number) || 0;
    const cost = (bucketData[`${combo.key}-cost`] as number) || 0;

    // 智能方向翻转：右侧空间不够则向左显示，下方空间不够则向上显示
    const viewportW = typeof window !== 'undefined' ? window.innerWidth : 1200;
    const viewportH = typeof window !== 'undefined' ? window.innerHeight : 800;
    const flipX = hoverInfo.clientX + ESTIMATED_TOOLTIP_WIDTH + 24 > viewportW;
    const flipY = hoverInfo.clientY + ESTIMATED_TOOLTIP_HEIGHT + 24 > viewportH;

    const left = flipX ? hoverInfo.clientX - ESTIMATED_TOOLTIP_WIDTH - 12 : hoverInfo.clientX + 12;
    const top = flipY ? hoverInfo.clientY - ESTIMATED_TOOLTIP_HEIGHT - 12 : hoverInfo.clientY + 12;

    return (
        <div
            className="border-border/50 bg-background grid gap-1.5 rounded-lg border px-3 py-2 text-xs shadow-xl max-w-xs pointer-events-none"
            style={{
                position: 'fixed',
                left: `${left}px`,
                top: `${top}px`,
                zIndex: 999,
            }}
        >
            <div className="font-medium text-foreground mb-0.5">
                {combo.channelName} / {combo.modelName}
            </div>
            <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-muted-foreground">
                <span>{t('tooltip.inputTokens')}：</span><span className="text-right font-mono">{formatNumber(inputTokens)}</span>
                <span>{t('tooltip.outputTokens')}：</span><span className="text-right font-mono">{formatNumber(outputTokens)}</span>
                <span>{t('tooltip.cacheReadTokens')}：</span><span className="text-right font-mono">{formatNumber(hitTokens)}</span>
                <span>{t('tooltip.cacheHitRate')}：</span><span className="text-right font-mono">{cacheHitRate.toFixed(2)}%</span>
                <span>{t('tooltip.callCount')}：</span><span className="text-right font-mono">{formatNumber(callCount)}</span>
                <span>{t('tooltip.cost')}：</span><span className="text-right font-mono">${formatCost(cost)}</span>
            </div>
        </div>
    );
}

// ==================== 图例 ====================

function CustomLegend({ combos }: { combos: Array<ComboInfo & { fill: string }> }) {
    if (combos.length === 0) return null;

    return (
        <div className="flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5 pt-3 pb-2">
            {combos.map((combo) => (
                <div key={combo.key} className="flex items-center gap-1.5">
                    <div className="h-2.5 w-2.5 shrink-0 rounded-sm" style={{ backgroundColor: combo.fill }} />
                    <span className="text-xs text-muted-foreground">{combo.channelName} / {combo.modelName}</span>
                </div>
            ))}
        </div>
    );
}

// ==================== 周期选项 ====================

const PERIOD_OPTIONS: Array<{ value: StatsPeriod; labelKey: string }> = [
    { value: 'hour', labelKey: 'period.hour' },
    { value: 'day', labelKey: 'period.day' },
    { value: 'week', labelKey: 'period.week' },
    { value: 'month', labelKey: 'period.month' },
];

// ==================== 主页面 ====================

export function StatsQuery() {
    const t = useTranslations('statsQuery');

    const committedParams = useStatsQueryStore();
    const submit = useStatsQueryStore((s) => s.submit);

    const [formStartTime, setFormStartTime] = useState<Date>(committedParams.startTime);
    const [formEndTime, setFormEndTime] = useState<Date>(committedParams.endTime);
    const [formChannelId, setFormChannelId] = useState<string>(
        committedParams.channelId !== undefined ? String(committedParams.channelId) : ''
    );
    const [formModelName, setFormModelName] = useState<string>(committedParams.modelName || '');
    const [formPeriod, setFormPeriod] = useState<StatsPeriod>(committedParams.period);

    // 渠道下拉选项 —— 通过 endpoint hook，无轮询
    const { data: channels } = useChannelOptions();

    const channelOptions = useMemo(() => {
        if (!channels) return [];
        return channels.map((ch) => ({
            value: String(ch.id),
            label: `${ch.name} (ID: ${ch.id})`,
        }));
    }, [channels]);

    // 模型下拉选项 —— 通过 endpoint hook，无轮询
    const { data: models } = useModelOptions();

    const modelOptions = useMemo(() => {
        if (!models) return [];
        return models.map((m) => ({ value: m.name, label: m.name }));
    }, [models]);

    const startTimeStr = dayjs(committedParams.startTime).format('YYYY-MM-DD HH:00');
    const endTimeStr = dayjs(committedParams.endTime).format('YYYY-MM-DD HH:00');

    const { data: chartData, isLoading, isFetching, error } = useStatsDetailChart({
        startTime: startTimeStr,
        endTime: endTimeStr,
        period: committedParams.period,
        channelId: committedParams.channelId,
        model: committedParams.modelName,
        _version: committedParams.queryVersion,
    });

    const querying = isLoading || isFetching;

    const allCombos = useMemo(() => {
        if (!chartData?.buckets) return [];
        const comboMap = new Map<string, ComboInfo>();
        for (const bucket of chartData.buckets) {
            for (const slot of bucket.slots) {
                const key = comboKey(slot.channel_id, slot.actual_model_name);
                if (!comboMap.has(key)) {
                    comboMap.set(key, {
                        key,
                        channelName: slot.channel_name || t('channelFallback', { id: slot.channel_id }),
                        modelName: slot.actual_model_name,
                    });
                }
            }
        }
        return Array.from(comboMap.values());
    }, [chartData, t]);

    const comboColors = useMemo(() => {
        return allCombos.map((combo, i) => ({
            ...combo,
            fill: seriesFill(i),
        }));
    }, [allCombos]);

    const rechartsData = useMemo(() => {
        if (!chartData?.buckets) return [];

        return chartData.buckets.map((bucket) => {
            const row: Record<string, number | string> = {
                time: formatTimeLabel(bucket.time, committedParams.period),
            };

            for (const combo of allCombos) {
                const slot = bucket.slots.find(
                    (s) => comboKey(s.channel_id, s.actual_model_name) === combo.key
                );
                if (slot) {
                    const totalTokens = slot.input_token + slot.output_token;
                    const cacheMiss = totalTokens - slot.cache_read_tokens;
                    row[`${combo.key}-miss`] = Math.max(0, cacheMiss);
                    row[`${combo.key}-hit`] = slot.cache_read_tokens;
                    row[`${combo.key}-input`] = slot.input_token;
                    row[`${combo.key}-output`] = slot.output_token;
                    row[`${combo.key}-calls`] = slot.api_call_count;
                    row[`${combo.key}-cost`] = slot.input_cost + slot.output_cost;
                } else {
                    row[`${combo.key}-miss`] = 0;
                    row[`${combo.key}-hit`] = 0;
                    row[`${combo.key}-input`] = 0;
                    row[`${combo.key}-output`] = 0;
                    row[`${combo.key}-calls`] = 0;
                    row[`${combo.key}-cost`] = 0;
                }
            }
            return row;
        });
    }, [chartData, allCombos, committedParams.period]);

    const xAxisInterval = useMemo(
        () => computeXAxisInterval(rechartsData.length),
        [rechartsData.length]
    );

    const handleQuery = useCallback(() => {
        if (formStartTime.getTime() >= formEndTime.getTime()) {
            toast.error(t('error.startAfterEnd'));
            return;
        }
        submit({
            startTime: formStartTime,
            endTime: formEndTime,
            channelId: formChannelId ? Number(formChannelId) : undefined,
            modelName: formModelName || undefined,
            period: formPeriod,
        });
    }, [formStartTime, formEndTime, formChannelId, formModelName, formPeriod, submit, t]);

    // 查询出错时记录日志
    useEffect(() => {
        if (error) {
            logger.warn('用量查询 API 请求失败:', error);
        }
    }, [error]);

    // ==================== 色块级 Hover 状态 ====================

    const [hoverInfo, setHoverInfo] = useState<HoverInfo | null>(null);

    /** 鼠标移入色块时记录 hover 信息，使用视口坐标用于 fixed 定位 */
    const handleSegmentHover = useCallback(
        (comboKey: string, bucketIndex: number, clientX: number, clientY: number) => {
            setHoverInfo({
                comboKey,
                bucketIndex,
                clientX,
                clientY,
            });
        },
        []
    );

    /** 鼠标移出色块时清除 hover */
    const handleSegmentLeave = useCallback(() => {
        setHoverInfo(null);
    }, []);

    /**
     * 为 Bar 创建自定义 shape 渲染函数。
     * hover 用 CSS filter brightness，不改 fill 字符串、无描边。
     */
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const createSegmentShape: any = useCallback(
        (comboKey: string, fill: string, isHit: boolean) => {
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            return (props: any) => {
                const idx: number = props.index;
                const isHovered = hoverInfo?.comboKey === comboKey && hoverInfo?.bucketIndex === idx;
                const p = props as { x: number; y: number; width: number; height: number };
                return (
                    <rect
                        x={p.x}
                        y={p.y}
                        width={p.width}
                        height={Math.max(0, p.height)}
                        fill={fill}
                        opacity={isHit ? CACHE_HIT_OPACITY : 1}
                        style={{
                            cursor: 'pointer',
                            transition: 'filter 0.15s',
                            filter: isHovered ? 'brightness(1.15)' : undefined,
                        }}
                        onMouseEnter={(e) => handleSegmentHover(comboKey, idx, e.clientX, e.clientY)}
                        onMouseLeave={handleSegmentLeave}
                    />
                );
            };
        },
        [hoverInfo, handleSegmentHover, handleSegmentLeave]
    );

    const showEmpty = !querying && (!chartData?.buckets || allCombos.length === 0);

    return (
        <PageWrapper className="h-full min-h-0 overflow-y-auto overscroll-contain space-y-6 pb-24 md:pb-4 rounded-t-3xl">
            {/* 查询表单 */}
            <div className="rounded-3xl bg-card border-card-border border p-4 custom-shadow">
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('startTime')}</Label>
                        <DateTimePicker value={formStartTime} onChange={setFormStartTime} />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('endTime')}</Label>
                        <DateTimePicker value={formEndTime} onChange={setFormEndTime} />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('period.label')}</Label>
                        <Select value={formPeriod} onValueChange={(v) => setFormPeriod(v as StatsPeriod)}>
                            <SelectTrigger className="w-full">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {PERIOD_OPTIONS.map((opt) => (
                                    <SelectItem key={opt.value} value={opt.value}>{t(opt.labelKey)}</SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('channel')}</Label>
                        <SearchableSelect
                            value={formChannelId}
                            onChange={setFormChannelId}
                            options={channelOptions}
                            placeholder={t('channelPlaceholder')}
                            noMatchText={t('searchNoMatch')}
                        />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('model')}</Label>
                        <SearchableSelect
                            value={formModelName}
                            onChange={setFormModelName}
                            options={modelOptions}
                            placeholder={t('modelPlaceholder')}
                            noMatchText={t('searchNoMatch')}
                        />
                    </div>
                    <div className="flex items-end">
                        <Button onClick={handleQuery} disabled={querying} className="w-full">
                            {querying ? t('querying') : t('query')}
                        </Button>
                    </div>
                </div>
                {error && (
                    <div className="mt-3 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                        {error.message || t('error.apiError')}
                    </div>
                )}
            </div>

            {/* 堆叠柱状图 */}
            <div className="rounded-3xl bg-card border-card-border border pt-4 pb-0 custom-shadow">
                <div className="px-4 pb-2">
                    <h3 className="font-semibold text-base">{t('chartTitle')}</h3>
                </div>

                {showEmpty ? (
                    <div className="flex items-center justify-center h-64 text-muted-foreground text-sm">
                        {t('noData')}
                    </div>
                ) : (
                    <div className="h-96 w-full px-2 relative">
                        <ResponsiveContainer width="100%" height="100%">
                            <BarChart data={rechartsData} barCategoryGap="15%">
                                <CartesianGrid strokeDasharray="3 3" vertical={false} />
                                <XAxis
                                    dataKey="time"
                                    tickLine={false}
                                    axisLine={false}
                                    tick={{ fontSize: 11 }}
                                    interval={xAxisInterval}
                                />
                                <YAxis tickLine={false} axisLine={false} tick={{ fontSize: 11 }} tickFormatter={formatNumber} />
                                {comboColors.map((combo) => (
                                    <Fragment key={combo.key}>
                                        <Bar
                                            dataKey={`${combo.key}-miss`}
                                            stackId="a"
                                            isAnimationActive={false}
                                            shape={createSegmentShape(combo.key, combo.fill, false)}
                                        />
                                        <Bar
                                            dataKey={`${combo.key}-hit`}
                                            stackId="a"
                                            isAnimationActive={false}
                                            shape={createSegmentShape(combo.key, combo.fill, true)}
                                        />
                                    </Fragment>
                                ))}
                            </BarChart>
                        </ResponsiveContainer>

                        {/* 色块级悬浮 Tooltip */}
                        <ComboTooltipOverlay
                            hoverInfo={hoverInfo}
                            rechartsData={rechartsData}
                            comboColors={comboColors}
                            t={t}
                        />
                    </div>
                )}

                <CustomLegend combos={comboColors} />
            </div>
        </PageWrapper>
    );
}
