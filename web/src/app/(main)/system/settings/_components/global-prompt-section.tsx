"use client";

import * as React from "react";

import { FileUpIcon, RotateCcwIcon, SaveIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";

type Config = { enabled: boolean; prompt: string };

const maxBytes = 128 * 1024;

export function GlobalPromptSection() {
  const [saved, setSaved] = React.useState<Config | null>(null);
  const [draft, setDraft] = React.useState<Config>({ enabled: false, prompt: "" });
  const [saving, setSaving] = React.useState(false);
  const [error, setError] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const fileInput = React.useRef<HTMLInputElement>(null);
  const bytes = new TextEncoder().encode(draft.prompt).length;
  const dirty = saved !== null && (draft.enabled !== saved.enabled || draft.prompt !== saved.prompt);
  const invalid = bytes > maxBytes || (draft.enabled && !draft.prompt.trim());
  let status = "未加载";
  if (loading) status = "读取中";
  else if (dirty) status = "有未保存更改";
  else if (saved) status = "已保存";

  const load = React.useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await api.getGlobalPrompt();
      setSaved(result);
      setDraft(result);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      const result = await api.saveGlobalPrompt(draft);
      setSaved(result);
      setDraft(result);
      toast.success("全局提示词已保存");
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const importFile = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (file.size > maxBytes) {
      setError("文件超过 128 KiB");
      return;
    }
    try {
      const prompt = await file.text();
      setDraft((current) => ({ ...current, prompt }));
      setError("");
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <CardTitle>全局 Agent 提示词</CardTitle>
            <CardDescription>作为正文前缀加入每次 Agent 运行；关闭开关会保留已填写内容。</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Label htmlFor="global-prompt-enabled">启用</Label>
            <Switch
              id="global-prompt-enabled"
              checked={draft.enabled}
              disabled={!saved || loading || saving}
              onCheckedChange={(enabled) => setDraft((current) => ({ ...current, enabled }))}
            />
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="global-prompt-text">正文</Label>
          <Textarea
            id="global-prompt-text"
            className="min-h-64 w-full resize-y font-mono text-sm"
            rows={14}
            value={draft.prompt}
            disabled={!saved || loading || saving}
            onChange={(event) => setDraft((current) => ({ ...current, prompt: event.target.value }))}
          />
          <div className="flex flex-wrap justify-between gap-2 text-muted-foreground text-xs" aria-live="polite">
            <span>{status}</span>
            <span className={bytes > maxBytes ? "text-destructive" : ""}>{bytes.toLocaleString()} / 131,072 字节</span>
          </div>
        </div>
        {error ? (
          <p role="alert" className="break-words text-destructive text-sm">
            {error}
          </p>
        ) : null}
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" size="sm" onClick={() => void save()} disabled={!dirty || invalid || loading || saving}>
            <SaveIcon />
            {saving ? "保存中" : "保存"}
          </Button>
          <Button
            type="button"
            size="icon"
            variant="outline"
            title="导入文本文件"
            aria-label="导入文本文件"
            disabled={!saved || loading || saving}
            onClick={() => fileInput.current?.click()}
          >
            <FileUpIcon />
          </Button>
          <Button
            type="button"
            size="icon"
            variant="outline"
            title="还原未保存内容"
            aria-label="还原未保存内容"
            disabled={!dirty || loading || saving}
            onClick={() => saved && setDraft(saved)}
          >
            <RotateCcwIcon />
          </Button>
          {!saved && !loading ? (
            <Button type="button" size="sm" variant="outline" onClick={() => void load()}>
              重试
            </Button>
          ) : null}
          <input
            ref={fileInput}
            type="file"
            accept=".md,.txt,text/plain,text/markdown"
            className="hidden"
            aria-label="提示词文件"
            onChange={(event) => void importFile(event)}
          />
        </div>
      </CardContent>
    </Card>
  );
}
