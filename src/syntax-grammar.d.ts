declare module "virtual:syntax-grammar/*" {
  const load: () => Promise<import("shiki/core").LanguageInput[]>;
  export default load;
}
