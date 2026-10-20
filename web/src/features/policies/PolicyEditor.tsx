import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { Bold, Heading2, Heading3, Italic, List, ListOrdered, Redo2, Undo2 } from 'lucide-react'
import { cn } from '@/lib/utils'

export function PolicyEditor({ html, onChange, editable }: { html: string; onChange: (html: string) => void; editable: boolean }) {
  const editor = useEditor({
    extensions: [StarterKit],
    content: html,
    editable,
    onUpdate: ({ editor }) => onChange(editor.getHTML()),
  })
  if (!editor) return null
  const btn = (active: boolean, onClick: () => void, Icon: typeof Bold, label: string) => (
    <button type="button" title={label} aria-label={label} onClick={onClick} className={cn('rounded p-1.5 hover:bg-accent', active && 'bg-accent text-foreground')}>
      <Icon className="h-4 w-4" />
    </button>
  )
  return (
    <div className="policy-editor rounded-md border">
      {editable ? (
        <div className="flex flex-wrap gap-1 border-b bg-muted/40 p-1 text-muted-foreground">
          {btn(editor.isActive('bold'), () => editor.chain().focus().toggleBold().run(), Bold, 'Bold')}
          {btn(editor.isActive('italic'), () => editor.chain().focus().toggleItalic().run(), Italic, 'Italic')}
          {btn(editor.isActive('heading', { level: 2 }), () => editor.chain().focus().toggleHeading({ level: 2 }).run(), Heading2, 'Heading')}
          {btn(editor.isActive('heading', { level: 3 }), () => editor.chain().focus().toggleHeading({ level: 3 }).run(), Heading3, 'Subheading')}
          {btn(editor.isActive('bulletList'), () => editor.chain().focus().toggleBulletList().run(), List, 'Bullet list')}
          {btn(editor.isActive('orderedList'), () => editor.chain().focus().toggleOrderedList().run(), ListOrdered, 'Numbered list')}
          <span className="mx-1 w-px bg-border" />
          {btn(false, () => editor.chain().focus().undo().run(), Undo2, 'Undo')}
          {btn(false, () => editor.chain().focus().redo().run(), Redo2, 'Redo')}
        </div>
      ) : null}
      <EditorContent editor={editor} className="policy-content p-4" data-testid="policy-editor" />
    </div>
  )
}
