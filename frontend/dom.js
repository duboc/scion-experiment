// Minimal DOM builder. Text is always inserted as text nodes, never parsed
// as HTML, so API data cannot inject markup (DESIGN.md §8 security rule).

/**
 * Creates an element.
 * @param {string} tag
 * @param {Record<string, string|boolean|number|null|undefined>} [attrs]
 *   Attribute values; false/null/undefined omit the attribute, true sets it empty.
 * @param {...(Node|string|number|null|undefined|false)} children
 *   Strings become text nodes; falsy children are skipped.
 */
export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [name, value] of Object.entries(attrs)) {
    if (name.startsWith('on')) {
      throw new Error(`el(): event handler attribute ${name} is not allowed; use addEventListener`);
    }
    if (value === false || value === null || value === undefined) continue;
    node.setAttribute(name, value === true ? '' : String(value));
  }
  append(node, ...children);
  return node;
}

/** Appends children, converting strings and numbers to text nodes. */
export function append(parent, ...children) {
  for (const child of children) {
    if (child === null || child === undefined || child === false) continue;
    parent.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return parent;
}

/** Replaces all children of parent. */
export function replaceChildren(parent, ...children) {
  parent.replaceChildren();
  return append(parent, ...children);
}

/** Sets the text of node; a no-op when unchanged to avoid needless DOM churn. */
export function setText(node, text) {
  const value = String(text);
  if (node.textContent !== value) node.textContent = value;
}
