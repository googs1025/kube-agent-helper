"use client";

import { useState, KeyboardEvent } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

interface TagInputProps {
  value: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
  className?: string;
  suggestions?: string[];
}

export function TagInput({ value, onChange, placeholder, className, suggestions }: TagInputProps) {
  const [input, setInput] = useState("");

  function addTag(tag: string) {
    const t = tag.trim();
    if (t && !value.includes(t)) onChange([...value, t]);
    setInput("");
  }

  function removeTag(tag: string) {
    onChange(value.filter((v) => v !== tag));
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(input);
    } else if (e.key === "Backspace" && input === "" && value.length > 0) {
      onChange(value.slice(0, -1));
    }
  }

  const remaining = suggestions?.filter((s) => !value.includes(s));

  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex min-h-9 flex-wrap items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-2 py-1.5 focus-within:border-blue-400 focus-within:ring-2 focus-within:ring-blue-500/20">
        {value.map((tag) => (
          <span key={tag} className="inline-flex items-center gap-1 rounded-full border border-blue-200 bg-blue-50 px-2 py-0.5 text-xs text-blue-700">
            {tag}
            <button type="button" onClick={() => removeTag(tag)} className="text-blue-400 hover:text-blue-700">
              <X size={10} />
            </button>
          </span>
        ))}
        {!suggestions && (
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKeyDown}
            onBlur={() => input && addTag(input)}
            placeholder={value.length === 0 ? placeholder : ""}
            className="min-w-[80px] flex-1 border-none bg-transparent text-sm outline-none placeholder:text-gray-400"
          />
        )}
      </div>
      {remaining && remaining.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {remaining.map((s) => (
            <button key={s} type="button" onClick={() => addTag(s)}
              className="rounded-full border border-gray-200 px-2 py-0.5 text-xs text-gray-500 hover:border-blue-300 hover:text-blue-600">
              + {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
