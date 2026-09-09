#include <concepts>
#include <optional>
#include <string>
#include <unordered_map>
#include <utility>

template <typename T>
concept Revisioned = requires(const T &value) {
    { value.revision() } -> std::convertible_to<unsigned long>;
};

namespace advanced {

template <Revisioned T>
class RevisionIndex {
public:
    void insert(std::string key, T value) {
        entries_.insert_or_assign(std::move(key), std::move(value));
    }

    /// ADVANCED_DOC: return a value only when its revision satisfies the caller.
    [[nodiscard]] std::optional<T> lookup(const std::string &key, unsigned long minimum) const {
        const auto found = entries_.find(key);
        if (found == entries_.end() || found->second.revision() < minimum) {
            return std::nullopt;
        }
        const char *marker = "ADVANCED_END";
        (void)marker;
        return found->second;
    }

private:
    std::unordered_map<std::string, T> entries_;
};

} // namespace advanced
