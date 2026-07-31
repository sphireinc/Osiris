function compareCodePoints(left, right) {
  const leftPoints = Array.from(left, (character) => character.codePointAt(0));
  const rightPoints = Array.from(right, (character) => character.codePointAt(0));
  const length = Math.min(leftPoints.length, rightPoints.length);

  for (let index = 0; index < length; index += 1) {
    if (leftPoints[index] !== rightPoints[index]) {
      return leftPoints[index] < rightPoints[index] ? -1 : 1;
    }
  }
  if (leftPoints.length === rightPoints.length) return 0;
  return leftPoints.length < rightPoints.length ? -1 : 1;
}

export function compareReferenceNames(left, right) {
  const folded = compareCodePoints(left.toLowerCase(), right.toLowerCase());
  return folded || compareCodePoints(left, right);
}

export function orderReferenceEntries(entries, pageID, sectionTitle) {
  const ordered = [...entries].sort((left, right) => compareReferenceNames(left.name, right.name));

  for (let index = 1; index < ordered.length; index += 1) {
    if (compareReferenceNames(ordered[index - 1].name, ordered[index].name) === 0) {
      throw new Error(
        `Duplicate reference entry sort identity on ${pageID}/${sectionTitle}: ${ordered[index].name}.`,
      );
    }
  }
  return ordered;
}

export function orderReferenceSections(sections, pageID) {
  return sections.map((section) => ({
    ...section,
    entries: orderReferenceEntries(section.entries, pageID, section.title),
  }));
}
